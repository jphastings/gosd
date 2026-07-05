// Package image writes flashable SD-card .img files: an MBR partition table
// with a FAT32 boot partition (and an optional second FAT32 data partition),
// built entirely in Go via github.com/diskfs/go-diskfs (no root, no external
// mkfs/fdisk tooling).
//
// The on-disk layout is locked:
//
//	byte 0        MBR (512 bytes)
//	byte 512      unpartitioned gap (Rockchip bootloaders land here on
//	              boards that need it - see the Radxa embed task)
//	byte 16MiB    partition 1: FAT32, type 0x0C, label GOSD-BOOT, 256MiB
//	byte 272MiB   partition 2 (optional): FAT32, type 0x0C, label GOSD-DATA,
//	              size from Spec.DataSizeBytes, immediately after partition 1
//	byte 272MiB+  end of image (or +Spec.DataSizeBytes if partition 2 exists)
//
// Partition 2 is omitted entirely (single-partition layout, unchanged from
// earlier versions) when Spec.DataSizeBytes is zero.
//
// Write is the only entry point; RawWrites into the gap and BootFiles into
// the FAT partition are both validated so a caller cannot accidentally
// clobber the MBR or either partition.
package image

import (
	"errors"
	"fmt"
	"io"
	"math"
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
	// fits inside the leading gap. This is the whole image when Spec's
	// data partition is disabled.
	totalImageSizeBytes = bootPartitionOffsetBytes + bootPartitionSizeBytes

	bootPartitionLabel      = "GOSD-BOOT"
	bootPartitionIndex      = 1
	bootPartitionStartLBA   = bootPartitionOffsetBytes / sectorSizeBytes
	bootPartitionSizeInLBAs = bootPartitionSizeBytes / sectorSizeBytes

	// dataPartitionOffsetBytes is the locked start of partition 2: directly
	// after partition 1, no gap.
	dataPartitionOffsetBytes = bootPartitionOffsetBytes + bootPartitionSizeBytes
	dataPartitionStartLBA    = dataPartitionOffsetBytes / sectorSizeBytes

	dataPartitionLabel = "GOSD-DATA"
	dataPartitionIndex = 2
)

// RawWrite is a raw byte write into the unpartitioned gap between the MBR
// and partition 1 (e.g. a board-specific bootloader). OffsetBytes is an
// absolute offset within the image file.
type RawWrite struct {
	OffsetBytes int64
	Content     io.Reader
}

// Spec describes the contents to write into a flashable SD-card image: the
// FAT32 boot partition's contents, any raw writes into the unpartitioned gap
// ahead of it, and the optional writable data partition.
type Spec struct {
	// BootFiles are files to place inside the FAT32 boot partition, keyed
	// by their path within that partition (forward-slash separated;
	// subdirectories are created as needed).
	BootFiles map[string]io.Reader

	// RawWrites are written directly into the unpartitioned gap between
	// the MBR and partition 1, after partitioning and formatting.
	RawWrites []RawWrite

	// DataSizeBytes is the size of the optional second partition (FAT32,
	// label GOSD-DATA, type 0x0C), created immediately after the boot
	// partition. Zero disables the partition entirely, producing the
	// single-partition layout (older images, or an explicit
	// --data-size=0). Non-zero sizes are rounded down to the nearest
	// whole sector.
	DataSizeBytes int64
}

// layout is the concrete geometry one Write call resolves Spec.DataSizeBytes
// into: whether partition 2 exists, and the image's total size.
type layout struct {
	totalSizeBytes          int64
	hasDataPartition        bool
	dataPartitionSizeInLBAs uint32
}

// computeLayout turns Spec.DataSizeBytes into a concrete layout, rejecting
// sizes that can't produce a valid partition.
func computeLayout(dataSizeBytes int64) (layout, error) {
	if dataSizeBytes < 0 {
		return layout{}, fmt.Errorf("data partition size %d bytes is negative", dataSizeBytes)
	}
	if dataSizeBytes == 0 {
		return layout{totalSizeBytes: totalImageSizeBytes}, nil
	}

	sizeInLBAs := dataSizeBytes / sectorSizeBytes
	if sizeInLBAs == 0 {
		return layout{}, fmt.Errorf("data partition size %d bytes is smaller than one sector (%d bytes)", dataSizeBytes, sectorSizeBytes)
	}
	if sizeInLBAs > math.MaxUint32 {
		return layout{}, fmt.Errorf("data partition size %d bytes is too large for an MBR partition", dataSizeBytes)
	}

	return layout{
		totalSizeBytes:          dataPartitionOffsetBytes + sizeInLBAs*sectorSizeBytes,
		hasDataPartition:        true,
		dataPartitionSizeInLBAs: uint32(sizeInLBAs),
	}, nil
}

// Write assembles a flashable MBR + FAT32 .img file at imgPath from spec.
// It is pure Go and requires no root privileges.
func Write(imgPath string, spec Spec) (err error) {
	lay, err := computeLayout(spec.DataSizeBytes)
	if err != nil {
		return fmt.Errorf("computing image layout for %s failed: %w", imgPath, err)
	}

	d, err := diskfs.Create(imgPath, lay.totalSizeBytes, diskfs.SectorSize512)
	if err != nil {
		return fmt.Errorf("creating image file %s failed: %w", imgPath, err)
	}
	defer func() {
		if cerr := d.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("closing image file %s failed: %w", imgPath, cerr)
		}
	}()

	partitions := []*mbr.Partition{
		{
			Index: bootPartitionIndex,
			Type:  mbr.Fat32LBA,
			Start: bootPartitionStartLBA,
			Size:  bootPartitionSizeInLBAs,
		},
	}
	if lay.hasDataPartition {
		partitions = append(partitions, &mbr.Partition{
			Index: dataPartitionIndex,
			Type:  mbr.Fat32LBA,
			Start: dataPartitionStartLBA,
			Size:  lay.dataPartitionSizeInLBAs,
		})
	}
	table := &mbr.Table{
		LogicalSectorSize:  sectorSizeBytes,
		PhysicalSectorSize: sectorSizeBytes,
		Partitions:         partitions,
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

	if lay.hasDataPartition {
		if _, err := d.CreateFilesystem(disk.FilesystemSpec{
			Partition:   dataPartitionIndex,
			FSType:      filesystem.TypeFat32,
			VolumeLabel: dataPartitionLabel,
		}); err != nil {
			return fmt.Errorf("formatting the %s FAT32 data partition failed: %w", dataPartitionLabel, err)
		}
	}

	if err := applyRawWrites(d, spec.RawWrites, lay); err != nil {
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
// OffsetBytes, refusing any write that would overlap the MBR, the boot
// partition, or (when present) the data partition.
func applyRawWrites(d *disk.Disk, writes []RawWrite, lay layout) error {
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

		if err := checkRawWriteBounds(w.OffsetBytes, int64(len(data)), lay); err != nil {
			return err
		}

		if _, err := wf.WriteAt(data, w.OffsetBytes); err != nil {
			return fmt.Errorf("raw write of %d bytes at offset %d failed: %w", len(data), w.OffsetBytes, err)
		}
	}

	return nil
}

// checkRawWriteBounds rejects a raw write of length bytes starting at offset
// if it would touch the MBR, the boot partition, or (when lay has one) the
// data partition, and if it would run past the end of the image entirely.
func checkRawWriteBounds(offset, length int64, lay layout) error {
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

	if lay.hasDataPartition && rangesOverlap(offset, end, dataPartitionOffsetBytes, lay.totalSizeBytes) {
		return fmt.Errorf("%w: write at offset %d (%d bytes) overlaps the %s data partition (bytes %d-%d); "+
			"raw writes must stay within the unpartitioned gap (bytes %d-%d)",
			ErrRawWriteOverlap, offset, length, dataPartitionLabel, dataPartitionOffsetBytes, lay.totalSizeBytes,
			mbrSizeBytes, bootPartitionOffsetBytes)
	}

	if end > lay.totalSizeBytes {
		return fmt.Errorf("write at offset %d (%d bytes) ends at byte %d, past the end of the %d-byte image",
			offset, length, end, lay.totalSizeBytes)
	}

	return nil
}

// rangesOverlap reports whether the half-open byte ranges [aStart, aEnd) and
// [bStart, bEnd) intersect.
func rangesOverlap(aStart, aEnd, bStart, bEnd int64) bool {
	return aStart < bEnd && bStart < aEnd
}
