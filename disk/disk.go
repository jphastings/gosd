// Package disk lets a GoSD app use an attached mass-storage disk — an NVMe SSD
// in an M.2 slot, a USB drive, an SD card in a reader — formatting it on first
// use and mounting it on every subsequent boot.
//
// It is the general-purpose sibling of the emmc package, which addresses one
// specific device (a board's soldered-on eMMC); disk takes whatever suitable
// mass storage it finds that the board did not boot from. The two have the same
// shape, and the same consequences: FormatAndMount writes a whole-device
// filesystem — no partition table — and is idempotent across runs, so once a
// disk carries a volume with the app's chosen label, later runs only mount it.
//
// ext4 is the default (Options.Filesystem's zero value, EXT4 — a deliberate
// breaking change from disk's earlier FAT32 default, ship notes in the
// release that carries it): it is journaled and crash-safe, which FAT32 and
// exFAT are neither, and internal drives are exactly where GoSD apps keep
// state that matters. FAT32 and exFAT remain available as explicit
// Options.Filesystem choices for the case ext4 does not serve — removable
// media meant to be read on another host, where FAT32's universal
// readability (or exFAT's, past FAT32's 4 GiB-per-file/256 GiB-volume
// ceilings) is the point, not a limitation. Whichever filesystem is in play,
// none of the three gives unix permissions or symlinks, and none but ext4's
// journal buys crash consistency for anything but its own metadata — write
// application data with the temp-file-then-rename pattern as for GOSD_DATA
// (see docs/runtime.md's fsync pattern).
//
// Discovery looks once by default, which suits an NVMe SSD or an eMMC — both
// already enumerated before an app's main runs. A USB drive can take seconds
// longer than that to appear, and Options.Wait is how long to keep looking for
// one.
//
// A disk already carrying a volume with the app's chosen label is mounted,
// not reformatted — but only when its filesystem also matches what was
// asked for. A label match against a *different* filesystem (a drive that
// arrived pre-formatted some other way, or an app whose Filesystem option
// changed across an upgrade) is treated like any other foreign content: see
// destructive, below.
//
// Formatting is destructive, so it is gated: FormatAndMount will format a blank
// disk freely, but refuses to overwrite anything else (a volume with a
// different label, or a filesystem GoSD cannot read) unless the caller
// explicitly opts in, returning an error wrapping ErrRefusedFormat otherwise.
package disk

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jphastings/gosd/internal/blockmount"
	"github.com/jphastings/gosd/internal/diskfmt"
)

// ErrNoDisk reports that no usable mass-storage disk was found: either nothing
// suitable is attached, or the only disk present is the one the board booted
// from and so is off-limits. Apps that can run without their disk should match
// this with errors.Is and carry on.
//
// A USB drive that is merely slow to enumerate reports this too, since from
// discovery's point of view it is indistinguishable from one that was never
// plugged in — see Options.Wait, which is how an app gives one time to appear.
var ErrNoDisk = errors.New("no usable disk found")

// ErrRefusedFormat reports that the disk already holds other content — a
// volume with a different label, or a filesystem GoSD cannot read — and
// destructive was false, so FormatAndMount left it untouched instead of wiping
// it. Callers that want to offer the user a way to consent (e.g. an app-env var
// read from the card's config/env/ settings) can match this with errors.Is and retry
// with destructive=true once they have it.
var ErrRefusedFormat = blockmount.ErrRefusedFormat

// ErrUnsupportedFS reports that the board's kernel cannot mount the filesystem
// the work needed — either one the caller asked for, or the one the disk
// already carries. Not every board's kernel has exFAT; this is reported before
// anything is written, so a caller can match it with errors.Is and fall back to
// FAT32 knowing the disk is untouched.
var ErrUnsupportedFS = blockmount.ErrUnsupportedFS

// ErrUnmountable reports the narrower case within ErrRefusedFormat: the disk
// already carries a volume labelled and formatted exactly as asked for, but
// it could not be mounted. Unlike ErrRefusedFormat's other causes, this is
// never someone else's data — it is the disk's own volume, unhealthy. The
// mount-failure refusal wraps both sentinels, so it never replaces
// ErrRefusedFormat and an existing errors.Is(err, ErrRefusedFormat) caller
// sees no change. An app that exposes one config flag authorizing adoption of
// a drive that isn't its own yet should gate on a second, differently-named
// flag before retrying with destructive=true on ErrUnmountable — that retry
// destroys the app's own data, not a stranger's.
var ErrUnmountable = blockmount.ErrUnmountable

// FormatAndMount ensures an attached disk carries an ext4 filesystem labelled
// label and mounts it read-write at mountpoint, then reports the outcome on the
// returned channel.
//
// It returns immediately; the work runs in the background so the caller can
// continue starting up. The channel receives exactly one Result and is then
// closed. A typical caller blocks on it only when it first needs the storage:
//
//	res := <-disk.FormatAndMount("APPDATA", "/storage", false)
//	if res.Err != nil {
//		log.Printf("no bulk storage: %v", res.Err)
//	}
//	// res.MountPoint is ready to use; res.BlockDevice is the node behind it.
//
// The disk is discovered automatically, once, with no wait for one that has
// yet to appear — see Devices for exactly which block devices qualify and in
// what order they are preferred, and Options.Wait for giving a USB drive that
// is still enumerating time to show up. A disk already carrying an ext4 volume
// with this label is only mounted, never reformatted (nor re-grown — growth
// happens exactly once, when the volume is first established), which is how
// re-runs of the same app avoid wiping their own data. A disk carrying a *different* filesystem under this label — e.g. one
// formatted by an app built before this default existed, or by a previous
// FormatAndMountWith(Options{Filesystem: ...}) choice — is treated as other
// content: see destructive, below. A blank disk (no filesystem and an
// all-zero leading region) is always formatted.
//
// destructive governs only a disk that already holds *other* data — content
// under a different label, or the same label under a different filesystem:
// false makes FormatAndMount fail without touching it, wrapping
// ErrRefusedFormat; true wipes and reformats it. label is limited to 16
// ASCII characters (ext4's limit; FormatAndMountWith with Options{Filesystem:
// FAT32} or ExFAT caps at 11 instead).
func FormatAndMount(label, mountpoint string, destructive bool) <-chan Result {
	return FormatAndMountWith(label, mountpoint, Options{Destructive: destructive})
}

// FormatAndMountDevice is FormatAndMount aimed at one named block device, e.g.
// "/dev/nvme0n1", for an app that has more than one disk attached and knows
// which it wants. Everything else is identical, including the refusal to touch
// a device that is currently in use — so naming the board's boot device by hand
// cannot wipe the running system.
func FormatAndMountDevice(device, label, mountpoint string, destructive bool) <-chan Result {
	return FormatAndMountWith(label, mountpoint, Options{Device: device, Destructive: destructive})
}

// Filesystem names an on-disk filesystem FormatAndMountWith can create.
type Filesystem string

const (
	// EXT4 is the default (Options.Filesystem's zero value) — a deliberate
	// breaking change from disk's earlier FAT32 default, called out in the
	// release notes of the version that shipped it. It is journaled and
	// crash-safe (see docs/runtime.md for what the journal does and does not
	// buy — metadata consistency and mount-time replay, not data
	// durability), at the cost of not being natively readable from macOS or
	// Windows and needing CONFIG_EXT4_FS in the board's kernel — see
	// COMPATIBILITY.md for which boards have it, which today is every board
	// GoSD ships. Asking for it on a board that lacks it fails with
	// ErrUnsupportedFS before the disk is touched. Formatting writes GoSD's
	// checked-in golden ext4 image (see internal/diskfmt), then grows it to
	// the disk's actual size exactly once, at establishment.
	EXT4 Filesystem = "ext4"
	// FAT32 is universal — every host mounts it, and every GoSD board's
	// kernel can — at the price of a hard 4 GiB ceiling on any single file,
	// however large the disk, a 256 GiB ceiling on the volume GoSD will
	// create (formatting a bigger disk fails with an error naming the
	// limit, leaving the disk untouched), and no crash safety. It remains
	// available for removable media meant to be read on another host,
	// which is the case ext4's default does not serve.
	FAT32 Filesystem = "fat32"
	// ExFAT lifts FAT32's two ceilings while staying widely host-readable,
	// at the cost of needing exFAT support in the board's kernel — see
	// COMPATIBILITY.md for which boards have it — and, like FAT32, no crash
	// safety. Asking for it on a board that lacks it fails with
	// ErrUnsupportedFS before the disk is touched.
	ExFAT Filesystem = "exfat"
)

// Options are the choices FormatAndMountWith makes available. Its zero value
// formats ext4, discovers the disk, and refuses to overwrite anything
// already there.
type Options struct {
	// Filesystem is what to format the disk as if formatting is needed. The
	// zero value is EXT4. It has no say over a disk that already carries a
	// volume with the app's label AND this filesystem: that is mounted, not
	// reformatted. A label match against a *different* filesystem is not
	// silently converted — see Destructive.
	Filesystem Filesystem
	// Device names one block device to use, e.g. "/dev/nvme0n1", for an app
	// with more than one disk attached. The zero value discovers one. A named
	// device that is in use is still refused.
	Device string
	// Destructive allows overwriting a disk that holds other content —
	// including a volume under the app's label whose filesystem does not
	// match Filesystem. The zero value refuses, wrapping ErrRefusedFormat.
	// It has no bearing on a blank disk, which is always formatted.
	Destructive bool
	// Wait is how long to keep looking for a disk that has not shown up yet
	// before giving up with ErrNoDisk. The zero value does not wait at all:
	// it looks once, which is right for the storage GoSD was first built
	// around — an NVMe SSD or an eMMC is on an on-SoC bus and is already
	// enumerated by the time an app's main runs.
	//
	// USB mass storage is the case that needs this. A stick or an enclosure
	// has to have its hub port powered, then be probed, then scanned, then
	// report its medium ready — commonly a second or two after the host
	// controller comes up, and longer through a hub or for a disk that has
	// to spin up. An app that reaches FormatAndMount before all that
	// finishes sees ErrNoDisk for a drive that is physically plugged in.
	// Setting Wait to a few seconds covers it; setting it to minutes turns
	// FormatAndMount into "use the drive whenever someone plugs one in",
	// which is the same gap seen from further away.
	//
	// There is deliberately no default window. An app that treats ErrNoDisk
	// as "no disk here, carry on" would be stalled by one, and a board with
	// nothing ever attached would pay it on every boot. Only the caller
	// knows which case it is in.
	//
	// Waiting never resolves the disk that gets formatted — it only answers
	// "has one shown up yet?", and discovery then runs normally. A drive
	// that appears and is claimed by something else in between still
	// reports ErrNoDisk.
	Wait time.Duration
}

// FormatAndMountWith is FormatAndMount with the choices spelled out — the
// filesystem to create, which disk to use, and whether other content may be
// overwritten:
//
//	res := <-disk.FormatAndMountWith("APPDATA", "/storage", disk.Options{
//		Filesystem:  disk.ExFAT,
//		Destructive: true,
//	})
//
// Everything else matches FormatAndMount, including returning immediately and
// delivering exactly one Result before closing the channel.
func FormatAndMountWith(label, mountpoint string, opts Options) <-chan Result {
	out := make(chan Result, 1)
	go func() {
		defer close(out)

		fs, err := opts.filesystem()
		if err != nil {
			out <- Result{Err: err}
			return
		}
		deps := newPlatformDeps()
		probe := func() error { _, err := discover(); return err }
		if opts.Device != "" {
			deps.Discover = func() (string, error) { return verifyNamedDevice(opts.Device) }
			probe = func() error { _, err := verifyNamedDevice(opts.Device); return err }
		}

		if err := awaitStorage(deps, probe, mountpoint, opts.Wait, time.Sleep); err != nil {
			out <- Result{Err: explainNoDisk(err, opts.Wait)}
			return
		}

		device, err := blockmount.Run(storage(deps), fs, label, mountpoint, opts.Destructive)
		if err != nil {
			out <- Result{Err: explainNoDisk(err, opts.Wait)}
			return
		}
		out <- Result{MountPoint: mountpoint, BlockDevice: device}
	}()
	return out
}

func (o Options) filesystem() (diskfmt.FS, error) {
	switch o.Filesystem {
	case "", EXT4:
		return diskfmt.EXT4, nil
	case FAT32:
		return diskfmt.FAT32, nil
	case ExFAT:
		return diskfmt.ExFAT, nil
	default:
		return "", fmt.Errorf("disk: %q is not a filesystem GoSD can create; use disk.EXT4, disk.FAT32 or disk.ExFAT", string(o.Filesystem))
	}
}

// discoveryPoll is how often awaitCandidate re-checks while waiting for a disk
// to appear. Enumerating a USB drive takes seconds, so this only has to be
// fast relative to a human noticing — reading /sys/block is cheap, but there is
// nothing to gain from spinning tighter.
const discoveryPoll = 250 * time.Millisecond

// awaitStorage holds FormatAndMountWith up until there is a disk for
// blockmount.Run to find, or Options.Wait runs out. A zero wait returns at
// once, leaving Run to discover exactly as it did before Wait existed — the
// point being that no app pays so much as an extra /sys/block read for an
// option it did not ask for.
//
// Waiting happens out here rather than inside deps.Discover because Run holds a
// process-wide lock across discovery that it shares with the emmc package
// (gosd-45bv): waiting under that lock would stall a sibling
// emmc.FormatAndMount for the whole window. Run still discovers for itself,
// under the lock, once a candidate has appeared — so this never decides which
// disk gets formatted, only when it is worth looking.
//
// The mounted check is not an optimisation. Run short-circuits a warm restart
// (the app relaunched without a reboot, storage still mounted) before it
// discovers anything, and a mounted disk is deliberately never a discovery
// candidate — so waiting for one to appear would spend the entire window and
// then fail with ErrNoDisk where Run would have succeeded immediately. An error
// reading the mount table is left for Run to report properly rather than
// guessed at here.
func awaitStorage(deps blockmount.Deps, probe func() error, mountpoint string, wait time.Duration, sleep func(time.Duration)) error {
	if wait <= 0 {
		return nil
	}
	if _, mounted, err := deps.MountedAt(mountpoint); err != nil || mounted {
		return nil
	}
	return awaitCandidate(probe, wait, sleep)
}

// awaitCandidate polls probe until it finds a disk or wait runs out, and
// reports probe's last error. A zero wait probes exactly once, which is what
// makes Options.Wait's zero value the behaviour that shipped before it existed.
//
// Only ErrNoDisk is worth retrying: it is the one error that means "not there
// (yet)" — including a disk that is present but currently in use, which may
// well be released. Anything else is a condition waiting cannot change (an
// unreadable /sys/block, a named device something has mounted), so it is
// returned at once rather than after the full window.
//
// sleep is a seam so the tests can drive the whole schedule without spending
// the wall-clock time; the real caller passes time.Sleep.
func awaitCandidate(probe func() error, wait time.Duration, sleep func(time.Duration)) error {
	err := probe()
	for remaining := wait; err != nil && remaining > 0 && errors.Is(err, ErrNoDisk); {
		nap := min(discoveryPoll, remaining)
		sleep(nap)
		remaining -= nap
		err = probe()
	}
	return err
}

// explainNoDisk adds the one remedy an ErrNoDisk might have to it. A disk that
// was still enumerating is the likeliest reason for a surprising ErrNoDisk on a
// board with a USB drive plugged in, and the fix is one option — so an app that
// never asked to wait is told it can, and one that did is told how long it
// waited, so the number can be raised knowingly. Other errors pass through
// untouched, and the wrapping keeps errors.Is(err, ErrNoDisk) true either way.
func explainNoDisk(err error, waited time.Duration) error {
	if err == nil || !errors.Is(err, ErrNoDisk) {
		return err
	}
	if waited <= 0 {
		return fmt.Errorf("%w; a USB drive can take a few seconds after boot to enumerate, so set disk.Options.Wait if one might not have appeared yet", err)
	}
	return fmt.Errorf("%w, after waiting %s for one to appear; raise disk.Options.Wait if the drive needs longer", err, waited)
}

// Result is the outcome of a FormatAndMount, delivered once on its channel. On
// success Err is nil and MountPoint/BlockDevice name the ready filesystem and
// the device behind it; on failure Err explains why and the other fields are
// empty.
type Result struct {
	// MountPoint is where the disk's filesystem is mounted read-write — the
	// mountpoint passed to FormatAndMount.
	MountPoint string
	// BlockDevice is the device node backing MountPoint, e.g. "/dev/nvme0n1".
	// The disk carries a whole-device filesystem (no partition table), so
	// this whole-device node can be handed straight to gadget.MassStorage to
	// share over USB — but Unmount MountPoint first: expose the device or mount
	// it, never both at once.
	BlockDevice string
	// Err is non-nil if the disk could not be formatted and mounted, including
	// ErrNoDisk when there is none and ErrRefusedFormat when it already holds
	// other content and destructive was false.
	Err error
}

// storage describes an attached disk to the shared orchestration.
func storage(d blockmount.Deps) blockmount.Storage {
	return blockmount.Storage{Pkg: "disk", Noun: "disk", Deps: d}
}

// deviceClasses are the block-device name prefixes that may be formatted, best
// first. Selection is an allowlist because /sys/block is full of nodes that
// would be catastrophic or pointless to format: loop* (files, not media),
// ram*/zram*/zd* (volatile RAM-backed), dm-* (device-mapper nodes — formatting
// one corrupts whatever it maps), md* (RAID members), sr*/scd* (optical),
// nbd* (network block devices) and mtdblock*/ubiblock* (raw-flash translation
// layers). The order prefers the deliberately-fitted, high-capacity device and
// leaves onboard MMC last, since the emmc package addresses that directly.
var deviceClasses = []string{
	"nvme", // NVMe namespaces, e.g. nvme0n1
	"sd",   // SCSI/USB mass storage, e.g. sda — USB sticks and enclosures
	"vd",   // virtio disks, e.g. vda
	"mmcblk",
}

// rank accepts a block device as a format target and orders it against the
// others. Beyond the class allowlist it rejects an eMMC's boot/RPMB/GP
// hardware partitions (which hold boot code, replay-protected data or
// vendor-managed content such as DRM keys and calibration — never general
// storage). Present-medium and write-protected exclusion is not this
// package's concern any more: blockmount.Usable enforces both, for every
// caller of Candidates/Choose, before rank is ever consulted — see gosd-ix38.
func rank(dev blockmount.Device) (int, bool) {
	if isMMCHardwarePartition(dev.Name) {
		return 0, false
	}
	for i, prefix := range deviceClasses {
		if hasClassPrefix(dev.Name, prefix) {
			return i, true
		}
	}
	return 0, false
}

// hasClassPrefix reports whether name belongs to a device class, requiring
// something after the prefix so a bare "sd" or "nvme" never matches.
func hasClassPrefix(name, prefix string) bool {
	return len(name) > len(prefix) && strings.HasPrefix(name, prefix)
}

// mmcHardwarePartitionRE matches the block-device names the kernel's MMC
// block driver registers for an eMMC's hardware partitions: boot0/boot1 (boot
// code), rpmb (replay-protected storage) and gp0-gp3 (vendor general-purpose
// areas — on a Rockchip board these typically hold DRM keys, calibration data
// or other secure storage the vendor put there, per gosd-f226). Each is its
// own /sys/block gendisk alongside the user-data area (e.g. mmcblk0gp0 next to
// mmcblk0), so it must be excluded structurally rather than by growing a
// suffix list: a suffix check risks a false positive against a plain
// partition name that happens to end the same way, and would need a new entry
// every time the kernel's naming grows. The digit groups use \d+ rather than
// a literal 0-3/0-1 so an unexpected shape (a double-digit device number, an
// index the kernel doesn't use today) is still caught defensively; the
// anchors keep a name that merely contains "boot"/"rpmb"/"gp" from matching
// by accident. Partitions of the user area (mmcblk0p1) are never mistaken for
// a hardware partition — "p1" is not one of "boot\d+", "rpmb" or "gp\d+" —
// though they are excluded from candidacy for other reasons (see rank).
var mmcHardwarePartitionRE = regexp.MustCompile(`^mmcblk\d+(boot\d+|rpmb|gp\d+)$`)

// isMMCHardwarePartition spots an eMMC's boot, replay-protected and
// general-purpose hardware partitions, which the kernel exposes as their own
// block devices alongside the user area.
func isMMCHardwarePartition(name string) bool {
	return mmcHardwarePartitionRE.MatchString(name)
}

// choose picks the disk to format from the block devices present: the
// best-ranked candidate the board is not currently running from, since a device
// with anything mounted from it is in use and never a format target.
func choose(devices []blockmount.Device, mountedSources map[string]bool) (string, error) {
	return blockmount.Choose(devices, mountedSources, rank, ErrNoDisk)
}

// candidates lists every device node that could be formatted, in the order
// FormatAndMount would prefer them.
func candidates(devices []blockmount.Device, mountedSources map[string]bool) []string {
	return blockmount.Candidates(devices, mountedSources, rank)
}

// verifyNamed checks a device a caller named explicitly. It skips the class
// allowlist — an explicit choice is an explicit choice — but keeps the in-use
// rule, which is the one that stops an app wiping the media it booted from.
func verifyNamed(device string, devices []blockmount.Device, mountedSources map[string]bool) (string, error) {
	for _, dev := range devices {
		if "/dev/"+dev.Name != device {
			continue
		}
		if blockmount.InUse(dev, mountedSources) {
			return "", fmt.Errorf("the disk at %s is in use — something is mounted from it, so formatting it would destroy a filesystem in use; unmount it first, or choose another of %v", device, candidates(devices, mountedSources))
		}
		return device, nil
	}
	return "", fmt.Errorf("%w: there is no block device %s attached; the usable ones are %v", ErrNoDisk, device, candidates(devices, mountedSources))
}
