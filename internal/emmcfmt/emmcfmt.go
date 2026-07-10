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

	"github.com/diskfs/go-diskfs"
	"github.com/diskfs/go-diskfs/disk"
	"github.com/diskfs/go-diskfs/filesystem"
)

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
