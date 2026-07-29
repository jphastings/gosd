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
	case contents.FS != "" && strings.EqualFold(contents.Label, label):
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
func ValidateLabel(pkg, label string) error {
	if label == "" {
		return fmt.Errorf("%s: the volume label must not be empty", pkg)
	}
	if len(label) > maxLabelLen {
		return fmt.Errorf("%s: volume label %q is %d characters; volume labels are at most %d", pkg, label, len(label), maxLabelLen)
	}
	for _, r := range label {
		if r > unicode.MaxASCII || !unicode.IsPrint(r) {
			return fmt.Errorf("%s: volume label %q must be printable ASCII", pkg, label)
		}
	}
	return nil
}
