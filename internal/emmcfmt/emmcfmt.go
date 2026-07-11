// Package emmcfmt is the spike (bean gosd-0s0m) proving that GoSD can format
// the onboard eMMC on the Rockchip boards on-device, in pure Go
// (CGO_ENABLED=0), with no external mkfs and no root beyond write access to
// the device node.
//
// The kernels on the Radxa Zero 3E and NanoPi Zero2 are VFAT-only, and the
// eMMC is soldered so it cannot be formatted on another machine, so a blank
// eMMC is unusable unless we can lay down a FAT filesystem here. This package
// shows that github.com/diskfs/go-diskfs — already used to build image files
// in internal/image — can instead target a real block device: diskfs.Open on
// a device node auto-detects its size via ioctl(BLKGETSIZE64), and
// CreateFilesystem with Partition 0 writes a whole-device FAT32 with no
// partition table (which also avoids the BLKRRPART reread that needs
// privileges).
//
// It is deliberately minimal. The production surface (format-if-blank + mount)
// belongs to the device package under bean gosd-tdcc; this package exists only
// to de-risk that work and will be folded into it.
package emmcfmt

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/diskfs/go-diskfs"
	"github.com/diskfs/go-diskfs/disk"
	"github.com/diskfs/go-diskfs/filesystem"
)

// blankProbeBytes is how much of the start of a device Inspect reads to decide
// whether it is "blank". It comfortably spans an MBR (sector 0), a GPT header
// and its primary entry array (sectors 1-33), and a FAT boot/reserved region,
// so any existing partition table or filesystem leaves a non-zero byte in it.
const blankProbeBytes = 1 << 20 // 1 MiB

// Contents describes what already occupies an eMMC device, which is all
// FormatAndMount's mount-only / format / refuse decision depends on.
type Contents struct {
	// IsFAT is true when the device carries a readable FAT filesystem
	// (FAT12/16/32).
	IsFAT bool

	// Label is that filesystem's volume label, trimmed of FAT's padding.
	// Meaningful only when IsFAT is true.
	Label string

	// Blank is true when the device has no readable filesystem and its
	// leading region is entirely zero — nothing to destroy, so it is safe to
	// format even without an explicit destructive opt-in. Meaningful only
	// when IsFAT is false.
	Blank bool
}

// Inspect reports what occupies the block device (or image file) at devicePath.
// It never modifies the device.
func Inspect(devicePath string) (Contents, error) {
	if fat, ok, err := inspectFAT(devicePath); err != nil {
		return Contents{}, err
	} else if ok {
		return fat, nil
	}

	blank, err := leadingRegionIsZero(devicePath)
	if err != nil {
		return Contents{}, err
	}
	return Contents{Blank: blank}, nil
}

// inspectFAT reports whether devicePath holds a FAT filesystem and, if so, its
// label. ok is false (with a nil error) when the device simply isn't FAT; a
// non-nil error means the device could not be read at all.
func inspectFAT(devicePath string) (contents Contents, ok bool, err error) {
	d, err := diskfs.Open(devicePath, diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		return Contents{}, false, fmt.Errorf("opening %s to inspect it failed: %w", devicePath, err)
	}
	defer func() { _ = d.Close() }()

	// GetFilesystem probes FAT32/16/12 (then other, non-FAT types); an error
	// or a non-FAT result both mean "not a FAT we recognise".
	fs, err := d.GetFilesystem(0)
	if err != nil || !isFAT(fs.Type()) {
		return Contents{}, false, nil
	}
	return Contents{IsFAT: true, Label: trimLabel(fs.Label())}, true, nil
}

func isFAT(t filesystem.Type) bool {
	return t == filesystem.TypeFat32 || t == filesystem.TypeFat16 || t == filesystem.TypeFat12
}

// trimLabel drops the trailing space/NUL padding FAT stores volume labels with.
func trimLabel(label string) string {
	return strings.TrimRight(label, " \x00")
}

// leadingRegionIsZero reports whether the first blankProbeBytes of devicePath
// (or all of it, if shorter) are entirely zero.
func leadingRegionIsZero(devicePath string) (bool, error) {
	f, err := os.Open(devicePath)
	if err != nil {
		return false, fmt.Errorf("opening %s to check if it is blank failed: %w", devicePath, err)
	}
	defer func() { _ = f.Close() }()

	buf := make([]byte, blankProbeBytes)
	n, err := io.ReadFull(f, buf)
	switch err {
	case nil, io.EOF, io.ErrUnexpectedEOF:
	default:
		return false, fmt.Errorf("reading the start of %s to check if it is blank failed: %w", devicePath, err)
	}
	for _, b := range buf[:n] {
		if b != 0 {
			return false, nil
		}
	}
	return true, nil
}

// FormatFAT32 formats the block device (or image file) at devicePath as a
// single whole-device FAT32 filesystem labelled volumeLabel, discarding any
// existing contents.
//
// It opens the device read-write without O_EXCL (diskfs' default open mode is
// exclusive, which fails when the kernel already holds a block device) and
// formats the whole device with no partition table, so no partition-table
// reread — the one step that needs privileges on real hardware — is performed.
// On a real Linux block device the size is detected automatically; the caller
// need not supply it.
func FormatFAT32(devicePath, volumeLabel string) (err error) {
	d, err := diskfs.Open(devicePath,
		diskfs.WithOpenMode(diskfs.ReadWrite),
		diskfs.WithSectorSize(diskfs.SectorSize512),
	)
	if err != nil {
		return fmt.Errorf("opening %s for formatting failed: %w", devicePath, err)
	}
	defer func() {
		if cerr := d.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("closing %s after formatting failed: %w", devicePath, cerr)
		}
	}()

	if _, err := d.CreateFilesystem(disk.FilesystemSpec{
		Partition:   0, // 0 = whole device, no partition table
		FSType:      filesystem.TypeFat32,
		VolumeLabel: volumeLabel,
	}); err != nil {
		return fmt.Errorf("writing a FAT32 filesystem to %s failed: %w", devicePath, err)
	}
	return nil
}
