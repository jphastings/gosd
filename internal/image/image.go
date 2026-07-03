// Package image writes flashable SD-card .img files: an MBR partition table
// with a single FAT32 boot partition, built entirely in Go via
// github.com/diskfs/go-diskfs (no root, no external mkfs/fdisk tooling).
//
// The on-disk layout is locked:
//
//	byte 0        MBR (512 bytes)
//	byte 512      unpartitioned gap (Rockchip bootloaders land here on
//	              boards that need it - see the Radxa embed task)
//	byte 16MiB    partition 1: FAT32, type 0x0C, label GOSD-BOOT, 256MiB
//	byte 272MiB   end of image
//
// Write is the only entry point; RawWrites into the gap and BootFiles into
// the FAT partition are both validated so a caller cannot accidentally
// clobber the MBR or the partition.
package image

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sort"

	"github.com/diskfs/go-diskfs"
	"github.com/diskfs/go-diskfs/disk"
	"github.com/diskfs/go-diskfs/filesystem"
	"github.com/diskfs/go-diskfs/partition/mbr"
)

const (
	sectorSizeBytes = 512

	// mbrSizeBytes is the region the MBR itself occupies, at the very
	// start of the image.
	mbrSizeBytes = sectorSizeBytes

	// bootPartitionOffsetBytes and bootPartitionSizeBytes are the locked
	// layout for partition 1: it starts at exactly 16MiB (LBA 32768 at
	// 512-byte sectors) and is 256MiB, leaving the 512B-16MiB gap
	// unpartitioned.
	bootPartitionOffsetBytes = 16 * 1024 * 1024
	bootPartitionSizeBytes   = 256 * 1024 * 1024

	// totalImageSizeBytes is 272MiB: the boot partition's offset plus its
	// size, with no additional slack needed since the MBR itself already
	// fits inside the leading gap.
	totalImageSizeBytes = bootPartitionOffsetBytes + bootPartitionSizeBytes

	bootPartitionLabel      = "GOSD-BOOT"
	bootPartitionIndex      = 1
	bootPartitionStartLBA   = bootPartitionOffsetBytes / sectorSizeBytes
	bootPartitionSizeInLBAs = bootPartitionSizeBytes / sectorSizeBytes
)

// RawWrite is a raw byte write into the unpartitioned gap between the MBR
// and partition 1 (e.g. a board-specific bootloader). OffsetBytes is an
// absolute offset within the image file.
type RawWrite struct {
	OffsetBytes int64
	Content     io.Reader
}

// Spec describes the contents to write into a flashable SD-card image: the
// FAT32 boot partition's contents, and any raw writes into the unpartitioned
// gap ahead of it.
type Spec struct {
	// BootFiles are files to place inside the FAT32 boot partition, keyed
	// by their path within that partition (forward-slash separated;
	// subdirectories are created as needed).
	BootFiles map[string]io.Reader

	// RawWrites are written directly into the unpartitioned gap between
	// the MBR and partition 1, after partitioning and formatting.
	RawWrites []RawWrite
}

// Write assembles a flashable MBR + FAT32 .img file at imgPath from spec.
// It is pure Go and requires no root privileges.
func Write(imgPath string, spec Spec) (err error) {
	d, err := diskfs.Create(imgPath, totalImageSizeBytes, diskfs.SectorSize512)
	if err != nil {
		return fmt.Errorf("creating image file %s failed: %w", imgPath, err)
	}
	defer func() {
		if cerr := d.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("closing image file %s failed: %w", imgPath, cerr)
		}
	}()

	table := &mbr.Table{
		LogicalSectorSize:  sectorSizeBytes,
		PhysicalSectorSize: sectorSizeBytes,
		Partitions: []*mbr.Partition{
			{
				Index: bootPartitionIndex,
				Type:  mbr.Fat32LBA,
				Start: bootPartitionStartLBA,
				Size:  bootPartitionSizeInLBAs,
			},
		},
	}
	if err := d.Partition(table); err != nil {
		return fmt.Errorf("writing the MBR partition table to %s failed: %w", imgPath, err)
	}

	fs, err := d.CreateFilesystem(disk.FilesystemSpec{
		Partition:   bootPartitionIndex,
		FSType:      filesystem.TypeFat32,
		VolumeLabel: bootPartitionLabel,
	})
	if err != nil {
		return fmt.Errorf("formatting the %s FAT32 boot partition failed: %w", bootPartitionLabel, err)
	}

	if err := writeBootFiles(fs, spec.BootFiles); err != nil {
		return err
	}

	if err := applyRawWrites(d, spec.RawWrites); err != nil {
		return err
	}

	return nil
}

// writeBootFiles copies each of files into the FAT32 filesystem fs, creating
// any parent directories the path requires.
func writeBootFiles(fs filesystem.FileSystem, files map[string]io.Reader) error {
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	for _, p := range paths {
		if p == "" {
			return errors.New("boot file path must not be empty")
		}

		if dir := path.Dir(p); dir != "." {
			if err := fs.Mkdir(dir); err != nil {
				return fmt.Errorf("creating boot partition directory %q failed: %w", dir, err)
			}
		}

		f, err := fs.OpenFile(p, os.O_CREATE|os.O_RDWR)
		if err != nil {
			return fmt.Errorf("creating boot partition file %q failed: %w", p, err)
		}
		if _, err := io.Copy(f, files[p]); err != nil {
			_ = f.Close()
			return fmt.Errorf("writing boot partition file %q failed: %w", p, err)
		}
		if err := f.Close(); err != nil {
			return fmt.Errorf("closing boot partition file %q failed: %w", p, err)
		}
	}

	return nil
}

// applyRawWrites writes each RawWrite's content into the image at its
// OffsetBytes, refusing any write that would overlap the MBR or partition 1.
func applyRawWrites(d *disk.Disk, writes []RawWrite) error {
	if len(writes) == 0 {
		return nil
	}

	wf, err := d.Backend.Writable()
	if err != nil {
		return fmt.Errorf("opening the image for raw writes failed: %w", err)
	}

	for _, w := range writes {
		if w.OffsetBytes < 0 {
			return fmt.Errorf("raw write offset %d is negative", w.OffsetBytes)
		}

		data, err := io.ReadAll(w.Content)
		if err != nil {
			return fmt.Errorf("reading raw write content for offset %d failed: %w", w.OffsetBytes, err)
		}

		if err := checkRawWriteBounds(w.OffsetBytes, int64(len(data))); err != nil {
			return err
		}

		if _, err := wf.WriteAt(data, w.OffsetBytes); err != nil {
			return fmt.Errorf("raw write of %d bytes at offset %d failed: %w", len(data), w.OffsetBytes, err)
		}
	}

	return nil
}

// checkRawWriteBounds rejects a raw write of length bytes starting at offset
// if it would touch the MBR or the boot partition, and if it would run past
// the end of the image entirely.
func checkRawWriteBounds(offset, length int64) error {
	end := offset + length

	if rangesOverlap(offset, end, 0, mbrSizeBytes) {
		return fmt.Errorf("%w: write at offset %d (%d bytes) overlaps the MBR (bytes 0-%d); "+
			"choose an offset at or after byte %d", ErrRawWriteOverlap, offset, length, mbrSizeBytes, mbrSizeBytes)
	}

	if rangesOverlap(offset, end, bootPartitionOffsetBytes, bootPartitionOffsetBytes+bootPartitionSizeBytes) {
		return fmt.Errorf("%w: write at offset %d (%d bytes) overlaps partition 1 (bytes %d-%d); "+
			"raw writes must stay within the unpartitioned gap (bytes %d-%d)",
			ErrRawWriteOverlap, offset, length, bootPartitionOffsetBytes, bootPartitionOffsetBytes+bootPartitionSizeBytes,
			mbrSizeBytes, bootPartitionOffsetBytes)
	}

	if end > totalImageSizeBytes {
		return fmt.Errorf("write at offset %d (%d bytes) ends at byte %d, past the end of the %d-byte image",
			offset, length, end, int64(totalImageSizeBytes))
	}

	return nil
}

// rangesOverlap reports whether the half-open byte ranges [aStart, aEnd) and
// [bStart, bEnd) intersect.
func rangesOverlap(aStart, aEnd, bStart, bEnd int64) bool {
	return aStart < bEnd && bStart < aEnd
}
