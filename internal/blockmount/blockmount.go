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

// ErrRefusedFormat reports that a device already holds other content — a FAT
// volume with a different label, or a non-FAT filesystem — and the caller did
// not opt into destroying it. Both public packages re-export this same value,
// so it means exactly one thing across GoSD.
var ErrRefusedFormat = errors.New("refusing to reformat")

// maxFATLabelLen is the FAT volume-label limit (11 bytes). FAT also stores
// labels upper-cased; the mount-only decision matches them case-insensitively
// so the label a caller passes round-trips regardless.
const maxFATLabelLen = 11

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
	// Format writes a whole-device FAT filesystem labelled label to device.
	Format func(device, label string) error
	// Mount mounts device read-write at mountpoint.
	Mount func(device, mountpoint string) error
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
// what is already present, whether to mount-only, format, or refuse. It returns
// the device node backing the mounted filesystem on success.
func Run(s Storage, label, mountpoint string, destructive bool) (string, error) {
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

	switch {
	case contents.IsFAT && strings.EqualFold(contents.Label, label):
		// Already provisioned by an earlier run — mount only.
	case contents.Blank:
		if err := s.Deps.Format(device, label); err != nil {
			return "", fmt.Errorf("formatting the blank %s at %s failed: %w", s.Noun, device, err)
		}
	case !destructive:
		return "", fmt.Errorf("the %s at %s already holds %s; %w it as %q without permission — pass destructive=true to wipe it", s.Noun, device, describe(contents), ErrRefusedFormat, label)
	default:
		if err := s.Deps.Format(device, label); err != nil {
			return "", fmt.Errorf("reformatting the %s at %s failed: %w", s.Noun, device, err)
		}
	}

	if err := s.Deps.Mount(device, mountpoint); err != nil {
		return "", fmt.Errorf("mounting the %s at %s onto %s failed: %w", s.Noun, device, mountpoint, err)
	}
	return device, nil
}

// describe renders what is on the device for the "refusing to reformat" error.
func describe(c diskfmt.Contents) string {
	switch {
	case c.IsFAT:
		return fmt.Sprintf("a FAT volume labelled %q", c.Label)
	case c.OtherFS != "":
		return fmt.Sprintf("an %s filesystem, which GoSD cannot mount", c.OtherFS)
	default:
		return "non-FAT content"
	}
}

// ValidateLabel checks label against FAT's volume-label rules, reporting any
// problem prefixed with the calling package's name.
func ValidateLabel(pkg, label string) error {
	if label == "" {
		return fmt.Errorf("%s: the volume label must not be empty", pkg)
	}
	if len(label) > maxFATLabelLen {
		return fmt.Errorf("%s: volume label %q is %d characters; FAT labels are at most %d", pkg, label, len(label), maxFATLabelLen)
	}
	for _, r := range label {
		if r > unicode.MaxASCII || !unicode.IsPrint(r) {
			return fmt.Errorf("%s: volume label %q must be printable ASCII", pkg, label)
		}
	}
	return nil
}
