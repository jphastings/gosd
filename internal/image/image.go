// Package image writes flashable SD-card .img files: an MBR partition table
// with a FAT32 boot partition (and an optional second FAT32 data partition),
// built entirely in Go via github.com/diskfs/go-diskfs (no root, no external
// mkfs/fdisk tooling).
//
// The on-disk layout is locked except for the boot partition's size, which is
// per-build (Spec.BootSizeBytes, defaulting to today's 256MiB - see
// `gosd build --boot-size`):
//
//	byte 0                      MBR (512 bytes)
//	byte 512                    unpartitioned gap (Rockchip bootloaders land
//	                            here on boards that need it - see the Radxa
//	                            embed task)
//	byte 16MiB                  partition 1: FAT32, type 0x0C, label
//	                            GOSD-BOOT, Spec.BootSizeBytes (default 256MiB)
//	byte 16MiB+BootSizeBytes    partition 2 (optional): FAT32, type 0x0C,
//	                            label GOSD-DATA, size from Spec.DataSizeBytes,
//	                            immediately after partition 1
//	end of image                (16MiB+BootSizeBytes, or +Spec.DataSizeBytes
//	                            if partition 2 exists)
//
// Partition 2 is omitted entirely (single-partition layout, unchanged from
// earlier versions) when Spec.DataSizeBytes is zero.
//
// The chosen boot size becomes part of an app's on-disk layout: a later
// build that changes it moves partition 2's start, so a device upgraded via
// plain reflash finds its old GOSD-DATA superblock at the wrong offset (or
// gone) and re-formats - a deliberate, documented consequence (see
// docs/design/upgrade-path.md §0.4 and §2), not a bug here.
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
	"strings"

	"github.com/diskfs/go-diskfs"
	"github.com/diskfs/go-diskfs/disk"
	"github.com/diskfs/go-diskfs/filesystem"
	"github.com/diskfs/go-diskfs/partition/mbr"

	"github.com/jphastings/gosd/internal/diskfmt"
)

const (
	sectorSizeBytes = 512

	// mbrSizeBytes is the region the MBR itself occupies, at the very
	// start of the image.
	mbrSizeBytes = sectorSizeBytes

	// bootPartitionOffsetBytes is the locked start of partition 1: exactly
	// 16MiB (LBA 32768 at 512-byte sectors), leaving the 512B-16MiB gap
	// unpartitioned. This does NOT move with Spec.BootSizeBytes - only the
	// partition's end (and so partition 2's start) does.
	bootPartitionOffsetBytes = 16 * 1024 * 1024

	// DefaultBootPartitionSizeBytes is partition 1's size when
	// Spec.BootSizeBytes is zero: today's locked constant (256MiB),
	// unchanged from before --boot-size existed. Exported so callers that
	// need to talk about "the default" (e.g. `gosd build --boot-size`'s
	// own flag default) don't duplicate the number.
	DefaultBootPartitionSizeBytes = 256 * 1024 * 1024

	bootPartitionLabel    = "GOSD-BOOT"
	bootPartitionIndex    = 1
	bootPartitionStartLBA = bootPartitionOffsetBytes / sectorSizeBytes

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
	// whole sector, and then to the largest size go-diskfs formats into a
	// self-consistent FAT32 volume (at most two clusters less).
	DataSizeBytes int64

	// BootSizeBytes is the size of the FAT32 boot partition (label
	// GOSD-BOOT), which always starts at byte 16MiB. Zero means
	// DefaultBootPartitionSizeBytes (256MiB, the size every image used
	// before `gosd build --boot-size` existed). Non-zero sizes are rounded
	// down to the nearest whole sector, and then to the largest size
	// go-diskfs formats into a self-consistent FAT32 volume (at most two
	// clusters less) - the same trim DataSizeBytes gets.
	BootSizeBytes int64
}

// WriteReport summarizes what a Write call actually wrote, so a caller can
// print a boot-volume usage line (`gosd build`'s per-board summary) without
// re-opening and re-inspecting the finished image.
type WriteReport struct {
	// BootPartitionSizeBytes is the boot partition's resolved capacity:
	// Spec.BootSizeBytes, or DefaultBootPartitionSizeBytes when that was
	// zero.
	BootPartitionSizeBytes int64

	// BootPartitionPayloadBytes is the sum of every BootFiles entry's
	// content length actually written. It is not the partition's true
	// on-disk usage - FAT32 cluster rounding, directory entries, and the
	// reserved boot region all add a little further overhead this doesn't
	// count - but it's close enough to watch headroom shrink release over
	// release.
	BootPartitionPayloadBytes int64
}

// layout is the concrete geometry one Write call resolves Spec.BootSizeBytes
// and Spec.DataSizeBytes into: the boot partition's real size, whether
// partition 2 exists, and the image's total size.
type layout struct {
	bootPartitionSizeBytes   int64
	bootPartitionSizeInLBAs  uint32
	dataPartitionOffsetBytes int64
	dataPartitionStartLBA    uint32
	totalSizeBytes           int64
	hasDataPartition         bool
	dataPartitionSizeInLBAs  uint32
}

// computeLayout turns Spec.BootSizeBytes and Spec.DataSizeBytes into a
// concrete layout, rejecting sizes that can't produce a valid partition.
func computeLayout(bootSizeBytes, dataSizeBytes int64) (layout, error) {
	if bootSizeBytes < 0 {
		return layout{}, fmt.Errorf("boot partition size %d bytes is negative", bootSizeBytes)
	}
	if bootSizeBytes == 0 {
		bootSizeBytes = DefaultBootPartitionSizeBytes
	}

	bootSizeInLBAs := bootSizeBytes / sectorSizeBytes
	if bootSizeInLBAs == 0 {
		return layout{}, fmt.Errorf("boot partition size %d bytes is smaller than one sector (%d bytes)", bootSizeBytes, sectorSizeBytes)
	}
	if bootSizeInLBAs > math.MaxUint32 {
		return layout{}, fmt.Errorf("boot partition size %d bytes is too large for an MBR partition", bootSizeBytes)
	}

	// go-diskfs lays a FAT32 volume out with a FAT too small to index every
	// cluster it advertises at ~0.8% of sizes, so - exactly like the data
	// partition below - the boot partition is trimmed to the largest size
	// go-diskfs formats correctly, which costs at most two clusters. The
	// 256MiB default (and every whole-MiB size in its cluster-size band) is
	// unaffected: this only ever bites within sectorsPerCluster+1 sectors of
	// a band's top.
	bootSizeInLBAs = diskfmt.LargestSelfConsistentFAT32Bytes(bootSizeInLBAs*sectorSizeBytes) / sectorSizeBytes
	bootSizeBytes = bootSizeInLBAs * sectorSizeBytes

	dataOffsetBytes := bootPartitionOffsetBytes + bootSizeBytes
	lay := layout{
		bootPartitionSizeBytes:   bootSizeBytes,
		bootPartitionSizeInLBAs:  uint32(bootSizeInLBAs),
		dataPartitionOffsetBytes: dataOffsetBytes,
		dataPartitionStartLBA:    uint32(dataOffsetBytes / sectorSizeBytes),
		totalSizeBytes:           dataOffsetBytes,
	}

	if dataSizeBytes < 0 {
		return layout{}, fmt.Errorf("data partition size %d bytes is negative", dataSizeBytes)
	}
	if dataSizeBytes == 0 {
		return lay, nil
	}

	sizeInLBAs := dataSizeBytes / sectorSizeBytes
	if sizeInLBAs == 0 {
		return layout{}, fmt.Errorf("data partition size %d bytes is smaller than one sector (%d bytes)", dataSizeBytes, sectorSizeBytes)
	}
	if sizeInLBAs > math.MaxUint32 {
		return layout{}, fmt.Errorf("data partition size %d bytes is too large for an MBR partition", dataSizeBytes)
	}

	// Same trim as the boot partition above.
	sizeInLBAs = diskfmt.LargestSelfConsistentFAT32Bytes(sizeInLBAs*sectorSizeBytes) / sectorSizeBytes

	lay.hasDataPartition = true
	lay.dataPartitionSizeInLBAs = uint32(sizeInLBAs)
	lay.totalSizeBytes = dataOffsetBytes + sizeInLBAs*sectorSizeBytes
	return lay, nil
}

// Write assembles a flashable MBR + FAT32 .img file at imgPath from spec. It
// is pure Go and requires no root privileges.
func Write(imgPath string, spec Spec) (report WriteReport, err error) {
	lay, err := computeLayout(spec.BootSizeBytes, spec.DataSizeBytes)
	if err != nil {
		return WriteReport{}, fmt.Errorf("computing image layout for %s failed: %w", imgPath, err)
	}

	d, err := diskfs.Create(imgPath, lay.totalSizeBytes, diskfs.SectorSize512)
	if err != nil {
		return WriteReport{}, fmt.Errorf("creating image file %s failed: %w", imgPath, err)
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
			Size:  lay.bootPartitionSizeInLBAs,
		},
	}
	if lay.hasDataPartition {
		partitions = append(partitions, &mbr.Partition{
			Index: dataPartitionIndex,
			Type:  mbr.Fat32LBA,
			Start: lay.dataPartitionStartLBA,
			Size:  lay.dataPartitionSizeInLBAs,
		})
	}
	table := &mbr.Table{
		LogicalSectorSize:  sectorSizeBytes,
		PhysicalSectorSize: sectorSizeBytes,
		Partitions:         partitions,
	}
	if err := d.Partition(table); err != nil {
		return WriteReport{}, fmt.Errorf("writing the MBR partition table to %s failed: %w", imgPath, err)
	}

	fs, err := d.CreateFilesystem(disk.FilesystemSpec{
		Partition:   bootPartitionIndex,
		FSType:      filesystem.TypeFat32,
		VolumeLabel: bootPartitionLabel,
	})
	if err != nil {
		return WriteReport{}, fmt.Errorf("formatting the %s FAT32 boot partition failed: %w", bootPartitionLabel, err)
	}

	payloadBytes, err := writeBootFiles(fs, spec.BootFiles)
	if err != nil {
		return WriteReport{}, wrapBootPartitionFullError(err, lay.bootPartitionSizeBytes)
	}

	if lay.hasDataPartition {
		if _, err := d.CreateFilesystem(disk.FilesystemSpec{
			Partition:   dataPartitionIndex,
			FSType:      filesystem.TypeFat32,
			VolumeLabel: dataPartitionLabel,
		}); err != nil {
			return WriteReport{}, fmt.Errorf("formatting the %s FAT32 data partition failed: %w", dataPartitionLabel, err)
		}
	}

	if err := applyRawWrites(d, spec.RawWrites, lay); err != nil {
		return WriteReport{}, err
	}

	return WriteReport{
		BootPartitionSizeBytes:    lay.bootPartitionSizeBytes,
		BootPartitionPayloadBytes: payloadBytes,
	}, nil
}

// bootPartitionFullMarker is the exact, unexported error text go-diskfs's
// FAT32 writer returns when a volume runs out of allocatable clusters
// (filesystem/fat12.FileSystem.allocateSpace, embedded into fat32.FileSystem)
// - the library exports no sentinel for it, so this substring match is the
// only way to recognize "the boot partition is full" and turn it into a
// refusal actionable at the flag that controls the partition's size, instead
// of a bare "no space left on device" a developer would otherwise have no way
// to connect to --boot-size.
const bootPartitionFullMarker = "no space left on device"

// wrapBootPartitionFullError recognizes go-diskfs's disk-full error inside
// err and wraps it into ErrBootPartitionFull with the partition's capacity,
// so a caller can add flag-specific guidance; any other error passes through
// unchanged.
func wrapBootPartitionFullError(err error, bootPartitionSizeBytes int64) error {
	if !strings.Contains(err.Error(), bootPartitionFullMarker) {
		return err
	}
	return fmt.Errorf("%w (capacity %d bytes): %w", ErrBootPartitionFull, bootPartitionSizeBytes, err)
}

// writeBootFiles copies each of files into the FAT32 filesystem fs, creating
// any parent directories the path requires, and returns the total bytes
// copied (WriteReport.BootPartitionPayloadBytes).
func writeBootFiles(fs filesystem.FileSystem, files map[string]io.Reader) (int64, error) {
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	var totalBytes int64
	for _, p := range paths {
		if p == "" {
			return totalBytes, errors.New("boot file path must not be empty")
		}

		if dir := path.Dir(p); dir != "." {
			if err := fs.Mkdir(dir); err != nil {
				return totalBytes, fmt.Errorf("creating boot partition directory %q failed: %w", dir, err)
			}
		}

		f, err := fs.OpenFile(p, os.O_CREATE|os.O_RDWR)
		if err != nil {
			return totalBytes, fmt.Errorf("creating boot partition file %q failed: %w", p, err)
		}
		n, err := io.Copy(f, files[p])
		totalBytes += n
		if err != nil {
			_ = f.Close()
			return totalBytes, fmt.Errorf("writing boot partition file %q failed: %w", p, err)
		}
		if err := f.Close(); err != nil {
			return totalBytes, fmt.Errorf("closing boot partition file %q failed: %w", p, err)
		}
	}

	return totalBytes, nil
}

// resolvedRawWrite is a RawWrite whose content has been read into memory, so
// its length - and therefore the byte range it occupies - is known.
type resolvedRawWrite struct {
	offsetBytes int64
	data        []byte
}

// applyRawWrites writes each RawWrite's content into the image at its
// OffsetBytes, refusing any write that would overlap the MBR, the boot
// partition, (when present) the data partition, or another RawWrite in
// writes. Each Content reader is read exactly once: RawWrite carries no
// length, so a write's byte range is only known once its content has been
// read, and content overlap can only be checked once every write's range is
// known - hence the two passes, read-and-check then write, rather than the
// read-check-write-per-item loop this replaced.
func applyRawWrites(d *disk.Disk, writes []RawWrite, lay layout) error {
	if len(writes) == 0 {
		return nil
	}

	resolved := make([]resolvedRawWrite, len(writes))
	for i, w := range writes {
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

		resolved[i] = resolvedRawWrite{offsetBytes: w.OffsetBytes, data: data}
	}

	if err := checkRawWritesDontOverlap(resolved); err != nil {
		return err
	}

	wf, err := d.Backend.Writable()
	if err != nil {
		return fmt.Errorf("opening the image for raw writes failed: %w", err)
	}

	for _, rw := range resolved {
		if _, err := wf.WriteAt(rw.data, rw.offsetBytes); err != nil {
			return fmt.Errorf("raw write of %d bytes at offset %d failed: %w", len(rw.data), rw.offsetBytes, err)
		}
	}

	return nil
}

// checkRawWritesDontOverlap rejects any pair of writes whose byte ranges
// intersect. checkRawWriteBounds already keeps each write clear of the MBR
// and the partitions individually, but has no visibility into sibling
// writes - two RawWrites that each individually land cleanly in the
// unpartitioned gap can still clobber each other (e.g. a board's
// idbloader.img growing far enough to run into its own u-boot.itb offset).
func checkRawWritesDontOverlap(writes []resolvedRawWrite) error {
	for i := 0; i < len(writes); i++ {
		iEnd := writes[i].offsetBytes + int64(len(writes[i].data))
		for j := i + 1; j < len(writes); j++ {
			jEnd := writes[j].offsetBytes + int64(len(writes[j].data))
			if rangesOverlap(writes[i].offsetBytes, iEnd, writes[j].offsetBytes, jEnd) {
				return fmt.Errorf("%w: raw write at offset %d (%d bytes, ends at byte %d) overlaps the raw write at "+
					"offset %d (%d bytes, ends at byte %d); shrink or move whichever one grew, or re-check the "+
					"board's locked raw-write offsets",
					ErrRawWriteOverlap, writes[i].offsetBytes, len(writes[i].data), iEnd,
					writes[j].offsetBytes, len(writes[j].data), jEnd)
			}
		}
	}

	return nil
}

// checkRawWriteBounds rejects a raw write of length bytes starting at offset
// if it would touch the MBR, the boot partition, or (when lay has one) the
// data partition, and if it would run past the end of the image entirely.
func checkRawWriteBounds(offset, length int64, lay layout) error {
	end := offset + length
	bootPartitionEndBytes := bootPartitionOffsetBytes + lay.bootPartitionSizeBytes

	if rangesOverlap(offset, end, 0, mbrSizeBytes) {
		return fmt.Errorf("%w: write at offset %d (%d bytes) overlaps the MBR (bytes 0-%d); "+
			"choose an offset at or after byte %d", ErrRawWriteOverlap, offset, length, mbrSizeBytes, mbrSizeBytes)
	}

	if rangesOverlap(offset, end, bootPartitionOffsetBytes, bootPartitionEndBytes) {
		return fmt.Errorf("%w: write at offset %d (%d bytes) overlaps partition 1 (bytes %d-%d); "+
			"raw writes must stay within the unpartitioned gap (bytes %d-%d)",
			ErrRawWriteOverlap, offset, length, bootPartitionOffsetBytes, bootPartitionEndBytes,
			mbrSizeBytes, bootPartitionOffsetBytes)
	}

	if lay.hasDataPartition && rangesOverlap(offset, end, lay.dataPartitionOffsetBytes, lay.totalSizeBytes) {
		return fmt.Errorf("%w: write at offset %d (%d bytes) overlaps the %s data partition (bytes %d-%d); "+
			"raw writes must stay within the unpartitioned gap (bytes %d-%d)",
			ErrRawWriteOverlap, offset, length, dataPartitionLabel, lay.dataPartitionOffsetBytes, lay.totalSizeBytes,
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
