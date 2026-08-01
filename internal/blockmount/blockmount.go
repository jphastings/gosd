// Package blockmount is the shared machinery behind the public emmc and disk
// packages: deciding, from what a block device already holds, whether to
// mount it as-is, format it, or refuse to touch it — and then doing so.
//
// Both public packages are thin parameterisations of this one. They differ only
// in what they call the storage in error messages, which sentinel they return
// when none is available, and which block devices they consider candidates;
// everything else (the orchestration, FAT's label rules, the boot-device
// exclusion, the Linux mount/sysfs primitives) is here so the two can never
// drift apart.
package blockmount

import (
	"errors"
	"fmt"
	"strings"
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

// maxLabelLen is the volume-label limit both FAT (11 bytes) and exFAT (11
// UTF-16 characters) impose. FAT also stores labels upper-cased; the mount-only
// decision matches them case-insensitively so the label a caller passes
// round-trips regardless.
const maxLabelLen = 11

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
}

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
func Run(s Storage, fs diskfmt.FS, label, mountpoint string, destructive bool) (string, error) {
	if err := ValidateLabel(s.Pkg, label); err != nil {
		return "", err
	}

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
	case labelMatches(contents, label):
		// Already provisioned by an earlier run — mount only, and mount it as
		// whatever filesystem it turned out to be. Reformatting it to the
		// caller's preferred filesystem would destroy the app's own data.
		format, fs = false, contents.FS
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

	if format {
		if err := s.Deps.Format(device, label, fs); err != nil {
			return "", fmt.Errorf("%s the %s at %s as %s failed: %w", action, s.Noun, device, fs, err)
		}
	}

	if err := s.Deps.Mount(device, mountpoint, fs); err != nil {
		return "", fmt.Errorf("mounting the %s at %s onto %s failed: %w", s.Noun, device, mountpoint, err)
	}
	return device, nil
}

// remedyFor is the actionable half of the unsupported-filesystem error. FAT32
// is every board's baseline, so it is worth suggesting for anything else — but
// suggesting it in place of itself would be nonsense.
func remedyFor(fs diskfmt.FS) string {
	if fs == diskfmt.FAT32 {
		return "rebuild the board's kernel with it (see docs/custom-kernels.md)"
	}
	return fmt.Sprintf("rebuild the board's kernel with it (see docs/custom-kernels.md), or use %s instead", diskfmt.FAT32)
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

// ValidateLabel checks label against the volume-label rules, reporting any
// problem prefixed with the calling package's name. The rules are FAT's, which
// is the stricter of the two filesystems GoSD writes — an 11-character
// printable-ASCII label is equally valid as an exFAT label, so one label works
// whichever filesystem the caller ends up with.
//
// A leading or trailing space is refused outright: both FAT32 and exFAT strip
// edge padding when they read a label back (see trimLabel), so such a label
// can never round-trip through format→Inspect. Undetected, that mismatch
// makes Run's idempotency check fail forever, reformatting — and destroying —
// the app's own data on every single boot. NUL is refused too, but by the
// printable-ASCII check below: it is a control character, not printable.
//
// A space at byte index 7 (fatShortNameSplit) is refused too, for labels
// longer than 8 bytes: go-diskfs's FAT directory-entry parser writes an
// 11-byte label as an 8-byte short-name field (bytes 0-7) followed by a
// 3-byte extension field (bytes 8-10), and trims trailing spaces off each
// field independently before concatenating them on read-back. For a label of
// 8 bytes or fewer, byte 7 is padding, and trimming it is correct — but for a
// longer label, byte 7 is the short-name field's own last byte, which that
// per-field trim cannot tell apart from padding, so a space there silently
// vanishes on read-back — the same round-trip failure and destroy-on-boot
// consequence as an edge space, just one byte position narrower than the
// edges the check above already catches. No other byte position is affected:
// the extension field (bytes 8-10) is only ever padded at its own trailing
// edge, which the existing edge-space rule already forbids landing on: see
// fatDirectoryEntryRoundTrip and gosd-f83b's exhaustive round-trip test.
// exFAT stores the label as a single contiguous UTF-16 run with no such
// split, so it is unaffected — confirmed alongside FAT32 in
// TestAllPositionsRoundTripOrAreRejected.
func ValidateLabel(pkg, label string) error {
	if label == "" {
		return fmt.Errorf("%s: the volume label must not be empty", pkg)
	}
	if len(label) > maxLabelLen {
		return fmt.Errorf("%s: volume label %q is %d characters; volume labels are at most %d", pkg, label, len(label), maxLabelLen)
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
	for _, r := range label {
		if r > unicode.MaxASCII || !unicode.IsPrint(r) {
			return fmt.Errorf("%s: volume label %q must be printable ASCII", pkg, label)
		}
	}
	return nil
}
