// Package blockmount is the shared machinery behind the public emmc and disk
// packages: deciding, from what a block device already holds, whether to
// mount it as-is, format it, or refuse to touch it — and then doing so.
//
// Both public packages are thin parameterisations of this one. They differ only
// in what they call the storage in error messages, which sentinel they return
// when none is available, and which block devices they consider candidates;
// everything else (the orchestration, the label rules, the boot-device
// exclusion, the Linux mount/sysfs primitives) is here so the two can never
// drift apart. Both emmc and disk pass a typed Filesystem token through to
// Run — emmc gained the same token disk already had (bean gosd-9sc4,
// mirroring gosd-1c0x/PR #192), both defaulting to EXT4 — so every code path
// below is reachable from either caller; neither package pins a single
// diskfmt.FS any more. What still differs between the two is candidate
// *selection*, not filesystem: emmc addresses exactly one device (chooseEMMC,
// keyed on Kind == "MMC") with no equivalent of disk's multi-class ranking or
// FormatAndMountDevice/Devices. The one part of that divergence that could
// destroy something — disk excluding an eMMC's boot/RPMB/GP hardware
// partitions explicitly, where emmc stayed away from them only by accident,
// via a sysfs-topology quirk — is closed: IsMMCHardwarePartition lives here
// and Usable applies it to every candidate of either package, so neither can
// pick one however its own Rank is written (bean gosd-b6jl, finishing what
// gosd-ix38 deliberately scoped out).
//
// Every volume this package creates is established through one crash-safe
// sequence — format → sync → mount → (grow, ext4 only) → marker — and a
// volume found on a later boot is adopted only against the marker that
// sequence writes last, never against a filesystem probe: a matching
// signature, a matching label, even a successful mount are none of them proof
// that a format ever finished (gosd-lirl's lesson; extending it from ext4 to
// FAT32 and exFAT is bean gosd-mm6q). That mirrors the write → sync → marker
// → sync convention every other on-disk commit in this codebase follows —
// cmd/gosd-init/internal/dataexpand's EstablishedMarker is the same idea
// applied to the SD card's own data partition. ext4 alone is also grown, once,
// at first establishment. See establish's doc for the full ordering argument,
// filesystem by filesystem, including the one deliberate exception: a
// FAT32/exFAT volume that predates the marker is adopted on the evidence of
// its own contents, because reformatting every card already in the field
// would be a far worse bug than the one the marker closes.
package blockmount

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"unicode"

	"github.com/jphastings/gosd/internal/diskfmt"
)

// ErrRefusedFormat reports that a device already holds other content — a
// volume with a different label, or a filesystem GoSD cannot read — and the
// caller did not opt into destroying it. Both public packages re-export this
// same value, so it means exactly one thing across GoSD.
var ErrRefusedFormat = errors.New("refusing to reformat")

// ErrUnsupportedFS reports that the board's kernel cannot mount the filesystem
// the work would need — either the one the caller asked to format, or the one
// the device already carries. It is reported before anything is written, so a
// caller that can fall back to another filesystem may match it with errors.Is
// and retry, knowing the device is untouched.
var ErrUnsupportedFS = errors.New("filesystem not supported by this board's kernel")

// ErrUnmountable reports the narrow case within ErrRefusedFormat where the
// device already carries a volume whose label AND filesystem both match what
// the caller asked for, but that volume could not be mounted — "this is
// yours, and it is sick", never a generic mount error and never foreign
// content (a different label, or the same label under a different
// filesystem, stay ErrRefusedFormat-only; see that sentinel). The
// mount-failure refusal wraps both sentinels together, so it never replaces
// ErrRefusedFormat: an existing errors.Is(err, ErrRefusedFormat) caller sees
// no change, while a caller that wants to ask consent for destroying its own
// data separately from adopting someone else's can match errors.Is(err,
// ErrUnmountable) instead.
//
// Its message is a sentence fragment, like ErrRefusedFormat's, so wrapping
// it costs the refusal no extra words: it stands in for the phrase the
// message already used rather than being appended to it. The serial console
// is the only user interface these boards have.
var ErrUnmountable = errors.New("it could not be mounted")

// maxLabelLen is the volume-label limit both FAT (11 bytes) and exFAT (11
// UTF-16 characters) impose. Every formatter in this stack stores a label's
// case exactly as given, and reads it back the same way; the mount-only
// decision still matches labels case-insensitively, so a volume relabelled
// by some other tool's upper-casing convention is recognised rather than
// destroyed.
const maxLabelLen = 11

// ext4MaxLabelLen mirrors internal/diskfmt's ext4LabelBytes: ext4's
// s_volume_name field is 16 raw bytes, wider than FAT's 11 and enforced
// independently here (rather than left for diskfmt.FormatEXT4 to discover)
// so a caller's label is rejected at the API boundary — before any device is
// touched — the same as every other label rule in this file.
const ext4MaxLabelLen = 16

// EstablishedMarker is an empty file EstablishMarker writes into the root of
// a volume this package has just formatted, fsynced and — for ext4 — grown,
// and that Run looks for before adopting an existing volume whose label and
// filesystem already match what was asked for (see establish). It exists
// because on none of the three filesystems is a probe-passing signature, or
// even a successful mount, proof that a format finished:
//
//   - ext4: diskfmt.FormatEXT4 streams a ~512MiB golden image onto the device
//     before this package ever sees it, and a crash partway through that
//     stream — or between the online grow and this marker — leaves a
//     filesystem that inspects, and often even mounts, perfectly fine over
//     truncated or unwritten backing data.
//   - FAT32 and exFAT: diskfmt never fsyncs mid-format (see
//     diskfmt.eraseLeadingRegion's doc, which states that policy), so until
//     the caller's own SyncDevice returns, ANY SUBSET of a format's writes
//     may have reached the medium — writeback order is not program order.
//     The volume label, which is all Run's adoption check used to read,
//     lives in the root directory (FAT32) or a root-directory entry (exFAT)
//     and says nothing whatever about whether the FATs, the allocation
//     bitmap or the up-case table underneath it are complete. Adopting on
//     the label alone hands an app torn cluster chains that corrupt on first
//     write; refusing to adopt because a half-written label did NOT land
//     wedges the storage forever, on every boot, with no data ever having
//     been at risk. Both were live bugs until bean gosd-mm6q.
//
// That is the same probe-vs-proof lesson dataexpand.EstablishedMarker's doc
// comment records, and the specific failure mode gosd-lirl's review caught: a
// probe is never proof a format completed. The marker means "the format, the
// sync that made it durable, the mount, and (for ext4) the grow all reached
// the medium, in that order" — see establish's doc for the full ordering
// argument.
//
// It is reserved: apps must leave it alone. A leading dot hides it from a
// casual `ls`; unlike dataexpand's FAT32 marker (written raw, unmounted, via
// go-diskfs, which cannot see a leading-dot name it writes — see
// diskfmt.CreateEmptyFile), this one is written through the kernel's own
// mounted filesystem, which has no such limitation on any of the three. The
// corollary is that diskfmt.RootFileExists can never be used to look for
// THIS marker on a FAT32 volume: only a mounted read finds it.
//
// Unlike dataexpand's marker, this one is not the *sole* gate: dataexpand's
// "established" record is an MBR partition-table entry, which nothing
// running on that partition can reach or delete, so its mere absence is
// unconditional proof nothing has ever been handed to an app. This marker
// lives inside the very filesystem it gates, reachable by anything with
// write access to the mounted volume — so establish treats its absence as a
// strong, but not sole, signal: see Deps.RootHasOtherContent for the second
// opinion that keeps an app's accidental (or deliberate) removal of this
// file — and, on FAT32/exFAT, every volume formatted before this marker
// existed at all — from looking identical to genuine crash debris.
const EstablishedMarker = ".gosd-established"

// Deps are the side-effecting operations Run needs, injected so the
// orchestration can be tested without real storage.
type Deps struct {
	// MountedAt reports whether something is already mounted at mountpoint,
	// and if so the device node backing it (so a warm restart can report the
	// device without re-discovering it — discovery deliberately skips mounted
	// devices).
	MountedAt func(mountpoint string) (device string, mounted bool, err error)
	// Discover returns the device node to use, or the package's
	// nothing-suitable-found sentinel.
	Discover func() (string, error)
	// Inspect reports what already occupies the device.
	Inspect func(device string) (diskfmt.Contents, error)
	// Format writes a whole-device filesystem of kind fs, labelled label, to
	// device.
	Format func(device, label string, fs diskfmt.FS) error
	// Mount mounts device read-write at mountpoint as a filesystem of kind fs.
	Mount func(device, mountpoint string, fs diskfmt.FS) error
	// Mountable reports whether this kernel can mount a filesystem of kind fs.
	Mountable func(fs diskfmt.FS) (bool, error)
	// MountedSources returns the set of device nodes currently mounted, e.g.
	// {"/dev/mmcblk1p1": true}. Run calls this a second time, immediately
	// before Format, to re-check that the device Discover chose is still
	// free — see the call site for why one check at Discover time is not
	// enough.
	MountedSources func() (map[string]bool, error)

	// SyncDevice flushes device's page-cache-buffered writes to the medium.
	// Called once, right after Format has written the new filesystem and
	// before the first Mount is asked to trust it. diskfmt never fsyncs
	// mid-format on any filesystem, so this call is the only thing that makes
	// a format durable — see establish's ordering argument.
	SyncDevice func(device string) error
	// EstablishMarker writes EstablishedMarker into the root of the
	// filesystem mounted at mountpoint and makes it durable, along with
	// everything written before it. Called exactly once, as the last step of
	// establishing a volume — see EstablishedMarker's doc. fs is passed
	// because what "durable" takes differs by filesystem (see the Linux
	// implementation).
	EstablishMarker func(mountpoint string, fs diskfmt.FS) error
	// MarkerEstablished reports whether the filesystem already mounted at
	// mountpoint carries EstablishedMarker.
	MarkerEstablished func(mountpoint string) (bool, error)
	// RootHasOtherContent reports whether the filesystem mounted at
	// mountpoint holds anything in its root directory beyond what a freshly
	// formatted volume of kind fs ships with (nothing at all for FAT32 and
	// exFAT; lost+found for diskfmt's golden ext4 image) — i.e., whether an
	// app has plausibly written real data here. Only consulted when
	// EstablishedMarker is absent: see establish's doc for why a missing
	// marker alone is neither proof that nothing of value is at risk (ext4)
	// nor proof that a format never finished (FAT32/exFAT, where volumes
	// predating the marker are still in the field).
	RootHasOtherContent func(mountpoint string, fs diskfmt.FS) (bool, error)
	// Unmount releases mountpoint. Only called when a volume that looked
	// adoptable (matching label and filesystem) turns out to be crash debris
	// rather than an established volume: establish unmounts it so the repair
	// can reformat the raw device underneath.
	Unmount func(mountpoint string) error

	// The field below is ext4-only: establish never calls it for FAT32 or
	// exFAT, so a Deps value that leaves it nil (any emmc or disk caller that
	// never asks for diskfmt.EXT4) is valid.

	// Grow expands the ext4 filesystem already mounted at mountpoint (backed
	// by device) to fill the device's actual size, via the kernel's
	// EXT4_IOC_RESIZE_FS ioctl. Called exactly once, immediately after the
	// first Mount of a freshly Format-ed volume — never again once
	// EstablishMarker has recorded that grow as done.
	Grow func(device, mountpoint string) error
}

// runMu serialises every call to Run, across both public packages: emmc and
// disk are thin parameterisations of this one function, so a process-wide
// lock here closes the race between them, not just within one. Discovery,
// inspection, formatting and mounting are once-per-boot operations on a
// human timescale (a format is seconds; everything else is instant), so
// serialising them costs nothing that matters — the alternative is two
// callers discovering the same idle device before either has mounted it,
// and both formatting it: guaranteed corruption (gosd-45bv). This mutex
// protects only against a sibling call to Run; it says nothing about a
// device being mounted by anything else in the meantime, which is what the
// MountedSources re-check right before Format is for.
var runMu sync.Mutex

// Storage is one kind of mass storage, as the shared orchestration sees it.
type Storage struct {
	// Pkg is the importing package's name, prefixing label-validation errors.
	Pkg string
	// Noun names the storage in error messages, e.g. "eMMC" or "disk", so an
	// app author reads about the thing they plugged in.
	Noun string
	// Deps are the operations to run.
	Deps Deps
}

// Run is the orchestration behind both packages' FormatAndMount: decide, from
// what is already present, whether to mount-only, format, or refuse. fs is the
// filesystem to create if formatting turns out to be necessary. It returns the
// device node backing the mounted filesystem on success.
//
// Adoption requires the existing volume's filesystem to match fs, not just
// its label: a label match against a *different* filesystem is treated the
// same as any other foreign content (destructive=false refuses with
// ErrRefusedFormat; destructive=true reformats as fs). Converting it
// silently, the way earlier versions of this function did, would hand back a
// volume the caller never asked for.
func Run(s Storage, fs diskfmt.FS, label, mountpoint string, destructive bool) (string, error) {
	if err := ValidateLabel(s.Pkg, fs, label); err != nil {
		return "", err
	}

	// See runMu's doc comment: this is the whole fix's first half. Held for
	// the rest of the function, including the mount at the very end, so a
	// second call to Run — for this device or any other — cannot even start
	// discovering until this one has either mounted its device (making it
	// show up as in-use to the next Discover) or failed.
	runMu.Lock()
	defer runMu.Unlock()

	// Warm restart (app relaunched without a reboot): the storage is still
	// mounted, so there is nothing to do but report the device behind it.
	if device, mounted, err := s.Deps.MountedAt(mountpoint); err != nil {
		return "", err
	} else if mounted {
		return device, nil
	}

	device, err := s.Deps.Discover()
	if err != nil {
		return "", err
	}

	contents, err := s.Deps.Inspect(device)
	if err != nil {
		return "", fmt.Errorf("inspecting the %s at %s failed: %w", s.Noun, device, err)
	}

	format, action := true, "formatting"
	switch {
	case labelMatches(contents, label) && contents.FS == fs:
		// Already provisioned by an earlier run, as the same filesystem —
		// mount only. Reformatting would destroy the app's own data.
		format = false
	case labelMatches(contents, label):
		// The label matches, but the filesystem does not: a drive that
		// arrived pre-formatted differently, or an app whose Filesystem
		// option changed across an upgrade. Never silently converted —
		// that would hand back a filesystem the caller never asked for —
		// so it is treated like any other foreign content.
		if !destructive {
			return "", fmt.Errorf("the %s at %s already holds %s, but %s was requested for it; %w it as %s without permission — pass destructive=true to reformat it as %s (or match the filesystem already there)",
				s.Noun, device, describe(contents), fs, ErrRefusedFormat, fs, fs)
		}
		action = "reformatting"
	case contents.Blank:
	case !destructive:
		return "", fmt.Errorf("the %s at %s already holds %s; %w it as %q without permission — pass destructive=true to wipe it", s.Noun, device, describe(contents), ErrRefusedFormat, label)
	default:
		action = "reformatting"
	}

	// Checked before any writing, so a kernel that cannot mount the filesystem
	// leaves the device exactly as it found it.
	if mountable, err := s.Deps.Mountable(fs); err != nil {
		return "", err
	} else if !mountable {
		return "", fmt.Errorf("the %s at %s needs %s, but this board's kernel has no %s support: %w — %s", s.Noun, device, fs, fs, ErrUnsupportedFS, remedyFor(fs))
	}

	return establish(s, device, fs, label, mountpoint, format, action, destructive)
}

// establish is Run's tail, reached once the label rules, the
// format-vs-mount-only decision and the kernel preflight have already
// passed: it either adopts the volume that is already there, or writes a new
// one through the crash-safe establishment sequence below, and hands back the
// mounted device either way.
//
// format and action are the format-vs-mount-only decision and its verb
// ("formatting"/"reformatting"), exactly as Run computed them — but
// format=false is only a tentative "this looks adoptable", confirmed or
// overturned below by the marker check. destructive is threaded through
// unchanged from Run: it governs the RootHasOtherContent fallback, and the
// mount-failure fallback right after it, the same way it governs every other
// "something is already here" decision in this package: when what is already
// on the device cannot be read well enough to tell debris from real data,
// GoSD refuses rather than guesses. Only a provably-empty device (Blank
// content, which never reaches this function with format=false — see Run's
// switch) or provably-never-handed-to-an-app debris (the no-marker,
// empty-root case below) is ever touched without destructive=true.
//
// # The establishment sequence, and what each step makes durable
//
//  1. Format writes the whole new filesystem to the raw device. diskfmt
//     fsyncs nothing mid-format, on any of the three filesystems — it says so
//     outright in diskfmt.eraseLeadingRegion's doc ("this package never
//     fsyncs mid-format; durability is the caller's job, once, at the end of
//     a whole establish sequence"). So at this point NOTHING is durable, and
//     no ordering among Format's own writes is durable either: writeback
//     order is not program order, so an arbitrary SUBSET of them may be on
//     the medium.
//  2. SyncDevice fsyncs the device node. When it returns, every byte Format
//     wrote is on the medium. This is the step that turns "the filesystem is
//     complete" from a hope into a fact, and the step the FAT32/exFAT path
//     did not have at all before bean gosd-mm6q.
//  3. Mount asks the kernel to interpret what step 2 has just made durable —
//     never something that could still evaporate.
//  4. Grow (ext4 only; EXT4_IOC_RESIZE_FS) is only meaningful against a
//     mounted filesystem, which step 3 just proved trustworthy. FAT32 and
//     exFAT are formatted to the device's real size in step 1 and have
//     nothing to grow.
//  5. EstablishMarker writes EstablishedMarker into the root and makes it
//     durable — see the Linux implementation for what that takes per
//     filesystem. It is the commit record: the one fact on the volume a
//     later boot is willing to treat as proof, and it is only written once
//     every step above it has already returned.
//
// # What an interruption leaves, and what the next boot does with it
//
// A crash before step 2 completes leaves an arbitrary subset of the format on
// the medium, so the next boot's Inspect can find any of:
//
//   - Nothing identifiable. FAT32's formatter erases the whole 1MiB window
//     Inspect probes before writing anything into it, and exFAT's rewrites
//     all of it (see diskfmt.eraseLeadingRegion), so a boot sector that never
//     landed leaves Contents.Blank — which Run formats freely, no consent
//     needed, nothing lost.
//   - The new filesystem under a label that is not the caller's. Both
//     formatters write the volume label into the root directory (go-diskfs
//     reads FAT32's label from there, not from the BPB), which is late in the
//     write set and easily the part that did not land. This is the one
//     interrupted-format outcome the marker cannot improve: an unlabelled or
//     wrongly-labelled volume is indistinguishable from a stranger's stick,
//     so it is refused without destructive=true exactly as it was before —
//     GoSD does not wipe volumes it cannot show are its own.
//   - The new filesystem under the caller's own label. This is the case the
//     marker settles, and the case that used to be adopted on the label
//     alone: no marker means step 5 never ran, which means this mountpoint
//     was never handed to an app, which means nothing here is worth keeping.
//
// A crash after step 5 completes leaves an established volume: the marker is
// durable only because everything before it already was.
//
// # FAT32 and exFAT volumes that predate the marker
//
// Cards flashed before this marker existed carry perfectly good FAT32 and
// exFAT volumes with no marker in them, and reformatting every one of them on
// the next boot would be a far worse bug than the one the marker closes. A
// legacy volume cannot be told apart from crash debris with certainty — both
// read as "my label, my filesystem, no marker" — so the two are separated on
// the one signal that does differ: what is in the root directory.
//
// A file in the root of a FAT32/exFAT volume can only have got there through
// a mounted filesystem, written by an app (or by a host, over USB mass
// storage) that was handed this mountpoint — which only ever happens after a
// format completed. Neither formatter creates a single file: go-diskfs zeroes
// the whole root-directory cluster before writing the label into it, exFAT
// writes a root directory holding only the label, bitmap and up-case entries,
// and the kernel lists none of those as directory entries. So content in the
// root is independent evidence that a format once finished, and such a volume
// is adopted — writing the marker as it goes, so the upgrade happens exactly
// once and a later boot needs no second opinion. An empty root is what an
// interrupted establishment leaves, and reformatting it destroys nothing that
// exists: no marker AND no files means nothing was ever written here.
//
// The deliberate asymmetry is that "no marker, root has files" ADOPTS on
// FAT32/exFAT where the identical state REFUSES on ext4. That is not an
// oversight:
//
//   - ext4 has a step after the format that nothing else records: the
//     one-time grow. Adopting an unmarked ext4 volume could hand an app a
//     512MiB golden image on a 1TB disk, permanently, so the marker's
//     absence must dominate there.
//   - ext4 has no legacy population to protect: its marker shipped with its
//     default (epic gosd-lfu0), so an unmarked ext4 volume holding content is
//     genuinely anomalous. FAT32 was GoSD's default for both packages long
//     before any of this existed.
//   - Refusing would not repair a FAT32/exFAT volume anyway — there is no
//     grow to have missed and no journal to replay — it would only make
//     storage that has been working for months unusable until a human
//     intervened, on a device with no interactive surface.
//
// The net effect on already-deployed cards is deliberately one-directional:
// every state this function adopts was already adopted before this bean (the
// old code adopted on the label alone), so nothing that used to be adopted
// stops being adopted except the one state that provably holds no files.
func establish(s Storage, device string, fs diskfmt.FS, label, mountpoint string, format bool, action string, destructive bool) (string, error) {
	if !format {
		// The label and filesystem both already match — mount it and look
		// for proof it was ever fully established.
		if mountErr := s.Deps.Mount(device, mountpoint, fs); mountErr == nil {
			established, err := s.Deps.MarkerEstablished(mountpoint)
			if err != nil {
				return "", fmt.Errorf("checking whether the %s at %s was already established failed: %w", s.Noun, mountpoint, err)
			}
			if established {
				// Adoption: mounted, matched, proven finished. No grow, no
				// reformat — growth happens exactly once, at establishment.
				return device, nil
			}
			// No marker. On its own this is ambiguous, unlike every other
			// crash-debris check in GoSD (e.g. dataexpand's identically
			// shaped one): dataexpand's gate is an MBR partition-table
			// entry, which an app running on that partition can never
			// reach or delete, so "no entry yet" is unconditional proof
			// nothing has ever been handed to an app. EstablishedMarker
			// is a plain file *inside* the filesystem it gates — an app
			// with a "clear my hidden files too" bug (or anything else that
			// removes it) can make an otherwise perfectly established,
			// data-bearing volume look identical to fresh debris on the
			// next boot, and on FAT32/exFAT every volume formatted before
			// this marker existed looks exactly the same way.
			// RootHasOtherContent is the second opinion that closes that
			// gap: a root directory holding nothing beyond what a fresh
			// format itself ships with is exactly what an interrupted
			// establishment leaves (an app can only write here after Run
			// has already handed the mountpoint back, which never happened
			// this run), so that case still self-heals with no consent
			// needed, same as blank media.
			used, err := s.Deps.RootHasOtherContent(mountpoint, fs)
			if err != nil {
				return "", fmt.Errorf("checking whether the %s at %s already holds real content failed: %w", s.Noun, mountpoint, err)
			}
			switch {
			case used && fs != diskfmt.EXT4:
				// A FAT32/exFAT volume with files in it but no marker: the
				// legacy card this function's doc argues about. Adopt it,
				// and record the marker so the next boot needs no second
				// opinion — the upgrade happens once, in place, and a
				// failure to write it costs only that (the volume is
				// adoptable on the evidence of its own contents either
				// way), so it must never fail an adoption that a full
				// volume, or one the kernel has remounted read-only, would
				// otherwise have completed exactly as before.
				_ = s.Deps.EstablishMarker(mountpoint, fs)
				return device, nil
			case used && !destructive:
				return "", fmt.Errorf("the %s at %s already holds a %s volume labelled %q with content in it, but no establishment marker (either an interrupted format, or something removed the marker); %w it without permission — pass destructive=true to overwrite it", s.Noun, device, fs, label, ErrRefusedFormat)
			case used:
				action = "reformatting"
			}
			if err := s.Deps.Unmount(mountpoint); err != nil {
				return "", fmt.Errorf("unmounting the unestablished %s at %s (to reformat it) failed: %w", s.Noun, mountpoint, err)
			}
		} else if !destructive {
			// Unlike the checks above, a mount failure leaves nothing
			// readable at all — there is no root directory to apply the
			// RootHasOtherContent second opinion to, and nothing was
			// mounted, so there is nothing to unmount either. The label and
			// filesystem still match what was asked for, which is exactly
			// what a volume holding real data that has become unmountable
			// through corruption or an unrelated hardware fault would also
			// look like from here — indistinguishable from debris without
			// being able to read it. Refuse rather than guess — and wrap
			// ErrUnmountable alongside ErrRefusedFormat, since this refusal
			// is specifically about the caller's own volume rather than
			// foreign content (see ErrUnmountable's doc).
			return "", fmt.Errorf("the %s at %s already carries a %s volume labelled %q, but %w (%v); it may hold real data that cannot be read to check, so %w it without permission — pass destructive=true to reformat it anyway", s.Noun, device, fs, label, ErrUnmountable, mountErr, ErrRefusedFormat)
		} else {
			action = "reformatting"
		}
	}

	// See Run's identical re-check for why this happens again immediately
	// before the write that would corrupt whatever mounted the device in
	// the meantime — including, here, a sibling process racing the unmount
	// above.
	if mountedSources, err := s.Deps.MountedSources(); err != nil {
		return "", err
	} else if mountedSources[device] {
		return "", fmt.Errorf("the %s at %s was mounted by something else while it was being prepared; refusing to format it — retry once whatever mounted it is done", s.Noun, device)
	}

	// The crash-safe establishment sequence — steps 1 to 5 of this function's
	// doc, which argues what each one makes durable and what an interruption
	// between any two of them leaves behind. A crash before the marker lands
	// leaves debris with no marker: the next boot's Inspect+Mount still finds
	// a label/filesystem match, finds no marker, and repeats this whole
	// sequence from the format — including ext4's grow, even if it had
	// already completed once, because the reformat has overwritten its result
	// with the pristine golden image regardless. That is correct, not merely
	// tolerated: the grow left no trace a future boot could trust either, so
	// redoing it costs time, never correctness.
	if err := s.Deps.Format(device, label, fs); err != nil {
		return "", fmt.Errorf("%s the %s at %s as %s failed: %w", action, s.Noun, device, fs, err)
	}
	if err := s.Deps.SyncDevice(device); err != nil {
		return "", fmt.Errorf("flushing the newly formatted %s at %s to the medium failed: %w", s.Noun, device, err)
	}
	if err := s.Deps.Mount(device, mountpoint, fs); err != nil {
		return "", fmt.Errorf("mounting the %s at %s onto %s failed: %w", s.Noun, device, mountpoint, err)
	}
	if fs == diskfmt.EXT4 {
		if err := s.Deps.Grow(device, mountpoint); err != nil {
			return "", fmt.Errorf("growing the newly formatted %s at %s to its partition size failed: %w", s.Noun, device, err)
		}
	}
	if err := s.Deps.EstablishMarker(mountpoint, fs); err != nil {
		return "", fmt.Errorf("recording the completed format on the %s at %s failed: %w", s.Noun, device, err)
	}
	return device, nil
}

// remedyFor is the actionable half of the unsupported-filesystem error. FAT32
// is every board's baseline, so it is worth suggesting for anything else — but
// suggesting it in place of itself would be nonsense.
func remedyFor(fs diskfmt.FS) string {
	switch fs {
	case diskfmt.FAT32:
		return "rebuild the board's kernel with it (see docs/custom-kernels.md)"
	case diskfmt.EXT4:
		// Deliberately doesn't enumerate boards: every board GoSD currently
		// ships builds CONFIG_EXT4_FS into its stock kernel (see
		// COMPATIBILITY.md's ext4 rows), so reaching this case means a
		// custom kernel dropped the option, or a future board's stock
		// kernel never had it — naming today's boards here would just go
		// stale the next time one changes either way.
		return "this board's kernel has no CONFIG_EXT4_FS; see COMPATIBILITY.md for which boards support ext4, rebuild the board's kernel with it (see docs/custom-kernels.md), or use FAT32 or exFAT instead"
	default:
		return fmt.Sprintf("rebuild the board's kernel with it (see docs/custom-kernels.md), or use %s instead", diskfmt.FAT32)
	}
}

// labelMatches reports whether contents already carries the label the caller
// is asking for. label is compared in its trimmed form even though
// ValidateLabel now refuses leading/trailing space outright — belt and
// braces, so that if that check were ever bypassed (a future call site, a
// regression) the comparison still can't mismatch forever: FAT32 and exFAT
// both strip an edge space on read-back, so comparing against the untrimmed
// caller string would otherwise reformat the device — and destroy the app's
// own data — on every single boot.
//
// The same belt-and-braces applies to ValidateLabel's other round-trip rule,
// the FAT-only byte-7 short-name/extension split (see fatShortNameSplit and
// fatDirectoryEntryRoundTrip): a bypassed label of that shape is also
// checked against what a real FAT32 read-back would actually produce.
func labelMatches(contents diskfmt.Contents, label string) bool {
	if contents.FS == "" {
		return false
	}
	if strings.EqualFold(contents.Label, trimLabel(label)) {
		return true
	}
	return contents.FS == diskfmt.FAT32 && strings.EqualFold(contents.Label, fatDirectoryEntryRoundTrip(label))
}

// trimLabel mirrors the edge padding both diskfmt readers strip on read-back
// (FAT32's and exFAT's label decoders each `TrimRight(label, " \x00")`),
// applied to both edges here since ValidateLabel refuses a leading space too.
func trimLabel(label string) string {
	return strings.Trim(label, " \x00")
}

// fatShortNameSplit is the 0-based byte offset of the short-name field's
// last byte within an 11-byte FAT 8.3 label — see ValidateLabel and
// fatDirectoryEntryRoundTrip for the mechanism this pins.
const fatShortNameSplit = 7

// fatDirectoryEntryRoundTrip simulates go-diskfs's FAT directory-entry
// parser (fat12.parseDirEntries) end to end, from a raw label as Format
// would write it: SetLabel right-pads the label to 11 bytes and writes it as
// an 8-byte short-name field followed by a 3-byte extension field; on
// read-back, the parser trims trailing spaces off each field independently
// before Label() concatenates them with no separator. Unlike trimLabel's
// edge-only approximation, this reproduces the exact on-disk transform, so
// it also catches the class ValidateLabel's byte-7 rule refuses: a space at
// byte index 7 that is real label content (for any label longer than 8
// bytes) rather than padding.
func fatDirectoryEntryRoundTrip(label string) string {
	if len(label) > maxLabelLen {
		label = label[:maxLabelLen]
	}
	padded := label + strings.Repeat(" ", maxLabelLen-len(label))
	short := strings.TrimRight(padded[:fatShortNameSplit+1], " ")
	ext := strings.TrimRight(padded[fatShortNameSplit+1:maxLabelLen], " ")
	return short + ext
}

// describe renders what is on the device for the "refusing to reformat" error.
func describe(c diskfmt.Contents) string {
	switch {
	case c.FS != "":
		return fmt.Sprintf("%s labelled %q", c.FS, c.Label)
	case c.OtherFS != "":
		return fmt.Sprintf("%s that GoSD could not read", c.OtherFS)
	default:
		return "content GoSD does not recognise"
	}
}

// ValidateLabel checks label against fs's volume-label rules, reporting any
// problem prefixed with the calling package's name.
//
// FAT32 and exFAT share the stricter of GoSD's rule sets — an 11-character
// printable-ASCII label with none of the round-trip hazards documented
// below — so that one label works whichever of the two the caller ends up
// with. ext4 has neither hazard (its s_volume_name field is NUL-padded, not
// space-padded, and is a single 16-byte field with no FAT-style short-name/
// extension split — see internal/diskfmt's trimEXT4Label), so only its own
// width (ext4MaxLabelLen) and printable-ASCII apply; a label ValidateLabel
// admits for ext4 need not also satisfy FAT's edge-space or byte-7 rules.
//
// A leading or trailing space is refused outright for FAT32/exFAT: both
// strip edge padding when they read a label back (see trimLabel), so such a
// label can never round-trip through format→Inspect. Undetected, that
// mismatch makes Run's idempotency check fail forever, reformatting — and
// destroying — the app's own data on every single boot. NUL is refused too,
// but by the printable-ASCII check below: it is a control character, not
// printable.
//
// A space at byte index 7 (fatShortNameSplit) is refused too for FAT32/
// exFAT, for labels longer than 8 bytes: go-diskfs's FAT directory-entry
// parser writes an 11-byte label as an 8-byte short-name field (bytes 0-7)
// followed by a 3-byte extension field (bytes 8-10), and trims trailing
// spaces off each field independently before concatenating them on
// read-back. For a label of 8 bytes or fewer, byte 7 is padding, and
// trimming it is correct — but for a longer label, byte 7 is the short-name
// field's own last byte, which that per-field trim cannot tell apart from
// padding, so a space there silently vanishes on read-back — the same
// round-trip failure and destroy-on-boot consequence as an edge space, just
// one byte position narrower than the edges the check above already
// catches. No other byte position is affected: the extension field (bytes
// 8-10) is only ever padded at its own trailing edge, which the existing
// edge-space rule already forbids landing on: see fatDirectoryEntryRoundTrip
// and gosd-f83b's exhaustive round-trip test. exFAT stores the label as a
// single contiguous UTF-16 run with no such split, so it is unaffected —
// confirmed alongside FAT32 in TestAllPositionsRoundTripOrAreRejected.
func ValidateLabel(pkg string, fs diskfmt.FS, label string) error {
	maxLen := maxLabelLen
	if fs == diskfmt.EXT4 {
		maxLen = ext4MaxLabelLen
	}

	if label == "" {
		return fmt.Errorf("%s: the volume label must not be empty", pkg)
	}
	if len(label) > maxLen {
		return fmt.Errorf("%s: volume label %q is %d characters; %s volume labels are at most %d", pkg, label, len(label), fs, maxLen)
	}
	for _, r := range label {
		if r > unicode.MaxASCII || !unicode.IsPrint(r) {
			return fmt.Errorf("%s: volume label %q must be printable ASCII", pkg, label)
		}
	}
	if fs == diskfmt.EXT4 {
		// Neither of FAT's round-trip hazards below applies: see the doc
		// comment above.
		return nil
	}
	if trimmed := strings.Trim(label, " "); trimmed != label {
		if trimmed == "" {
			return fmt.Errorf("%s: volume label %q is all spaces; pick a label with real characters in it", pkg, label)
		}
		return fmt.Errorf("%s: volume label %q has a leading or trailing space; FAT32 and exFAT both strip it when reading the label back, so the device would look re-labelled on every boot — remove the space (or use %q)", pkg, label, trimmed)
	}
	if len(label) > fatShortNameSplit+1 && label[fatShortNameSplit] == ' ' {
		return fmt.Errorf("%s: volume label %q has a space as its 8th character; FAT32 stores an 11-byte label as an 8-byte name plus a 3-byte extension and trims trailing spaces from each independently when reading it back, so this space is indistinguishable from padding and silently disappears — move it, remove it, or replace it with a non-space character", pkg, label)
	}
	return nil
}
