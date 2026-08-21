// Package image writes flashable SD-card .img files: an MBR partition table
// with a FAT32 boot partition and an optional second data partition (FAT32
// or ext4). The FAT32 work is built entirely in Go via
// github.com/diskfs/go-diskfs (no root, no external mkfs/fdisk tooling); an
// ext4 data partition instead uses internal/diskfmt's pure-Go golden-image
// writer.
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
//	                            Spec.BootLabel, Spec.BootSizeBytes (default
//	                            256MiB)
//	byte 16MiB+BootSizeBytes    partition 2 (optional): label Spec.DataLabel,
//	                            size from Spec.DataSizeBytes, immediately
//	                            after partition 1; type 0x0C (FAT32, the
//	                            default) or 0x83 (ext4), per
//	                            Spec.DataFilesystem
//	end of image                (16MiB+BootSizeBytes, or +Spec.DataSizeBytes
//	                            if partition 2 exists)
//
// Partition 2 is omitted entirely (single-partition layout, unchanged from
// earlier versions) when Spec.DataSizeBytes is zero. An ext4 partition 2
// ships only a fixed ~512MiB golden filesystem (diskfmt.WriteEXT4) regardless
// of the partition's real size - growing it to fill the partition is a
// first-boot runtime step (EXT4_IOC_RESIZE_FS,
// internal/blockmount.GrowEXT4), entirely outside this package.
//
// The chosen boot size becomes part of an app's on-disk layout: a later
// build that changes it moves partition 2's start, so a device upgraded via
// plain reflash finds its old data-partition superblock at the wrong offset
// (or gone) and re-formats - a deliberate, documented consequence (see
// docs/design/upgrade-path.md §0.4 and §2), not a bug here. The labels are
// in that same category: they're per-app (`gosd build --label-prefix`), and
// gosd-init only adopts a surviving data partition whose label matches the
// one this image was built with.
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
	"github.com/diskfs/go-diskfs/filesystem/fat12"
	"github.com/diskfs/go-diskfs/partition/mbr"

	"github.com/jphastings/gosd/internal/diskfmt"
)

const (
	sectorSizeBytes = 512

	// SectorSizeBytes mirrors the unexported sectorSizeBytes above,
	// exported so a caller that must reject a too-small size before Write
	// ever sees it (e.g. cmd/gosd's --data-size floor check, which fails
	// fast rather than letting a sub-sector size survive a full build
	// only to be refused here by computeLayout) can name the same
	// boundary this package enforces internally, instead of duplicating
	// the number.
	SectorSizeBytes = sectorSizeBytes

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

	bootPartitionIndex    = 1
	bootPartitionStartLBA = bootPartitionOffsetBytes / sectorSizeBytes

	dataPartitionIndex = 2

	// maxLabelLen is FAT's 11-byte volume-label limit, the only label rule
	// this package enforces itself (see checkLabels): every other rule -
	// charset, spaces, the 8th-byte hazard - is
	// internal/blockmount.ValidateLabel's, applied at the CLI boundary where
	// a bad value is a user's typo rather than a programming error.
	maxLabelLen = 11
)

// RawWrite is a raw byte write into the unpartitioned gap between the MBR
// and partition 1 (e.g. a board-specific bootloader). OffsetBytes is an
// absolute offset within the image file.
type RawWrite struct {
	OffsetBytes int64
	Content     io.Reader
}

// ByteRange is a contiguous absolute byte range within the image file.
type ByteRange struct {
	OffsetBytes, LengthBytes int64
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

	// BootLabel and DataLabel are the volume labels partition 1 and
	// partition 2 are formatted with - per-app, so a flashed card appears
	// on a person's desktop named after the app rather than after GoSD
	// (`gosd build --label-prefix`, see internal/naming.LabelsFor).
	//
	// Both are required, with no default: a caller that has resolved a
	// label pair once, at the CLI boundary, cannot then accidentally ship
	// an image labelled something else, and gosd-init's adoption gate
	// compares against exactly the label baked in beside it (see
	// internal/initcfg.Config.DataLabel). Write refuses an empty or
	// over-long label before it creates the image file at all; DataLabel
	// is ignored (and needn't be set) when DataSizeBytes is zero, since
	// there is no partition 2 to label.
	BootLabel, DataLabel string

	// DataSizeBytes is the size of the optional second partition (labelled
	// DataLabel), created immediately after the boot partition, in the
	// filesystem DataFilesystem selects. Zero disables the partition
	// entirely, producing the single-partition layout (older images, or
	// an explicit --data-size=0). Non-zero sizes are rounded down to the
	// nearest whole sector. When DataFilesystem is FAT32 (the default),
	// sizes are then further trimmed to the largest size go-diskfs
	// formats into a self-consistent FAT32 volume (at most two clusters
	// less) - the same trim BootSizeBytes gets. That trim is a
	// workaround for a go-diskfs defect and does NOT apply to ext4; an
	// ext4 DataSizeBytes must instead be at least
	// diskfmt.EXT4GoldenData.MinBytes(), or Write refuses.
	DataSizeBytes int64

	// DataFilesystem selects the filesystem written into the data
	// partition. The zero value ("") means diskfmt.FAT32 (today's
	// default, unchanged). diskfmt.EXT4 writes a crash-resilient ext4
	// filesystem instead, via diskfmt.WriteEXT4 - see that function's doc
	// comment for what "written" does and does not mean yet (it ships a
	// fixed ~512MiB golden filesystem; growing it to the partition's real
	// size is a first-boot runtime step, outside this package). Only
	// diskfmt.FAT32 and diskfmt.EXT4 are accepted - diskfmt.ExFAT is
	// deliberately not supported here yet - any other value is a Write-time
	// error naming this field and its accepted values. Meaningless (and
	// ignored) when DataSizeBytes is zero: there is no data partition to
	// format at all.
	DataFilesystem diskfmt.FS

	// BootSizeBytes is the size of the FAT32 boot partition (labelled
	// BootLabel), which always starts at byte 16MiB. Zero means
	// DefaultBootPartitionSizeBytes (256MiB, the size every image used
	// before `gosd build --boot-size` existed). Non-zero sizes are rounded
	// down to the nearest whole sector, and then to the largest size
	// go-diskfs formats into a self-consistent FAT32 volume (at most two
	// clusters less) - the same trim DataSizeBytes gets.
	BootSizeBytes int64

	// ReportRanges lists the boot files whose on-disk byte ranges the
	// caller wants reported back in WriteReport.FileRanges - so a
	// provisioning tool can splice same-length replacement bytes into a
	// placeholder file, or into one of the config tree's value files,
	// without any FAT tooling (see internal/inject and
	// docs/image-injection.md). Each entry is a whole file, keyed as in
	// BootFiles; every entry must name a key of BootFiles, and no path may
	// appear twice; Write checks both up front, before any image bytes
	// exist.
	ReportRanges []string
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

	// FileRanges holds, for each of Spec.ReportRanges, the ordered,
	// absolute, exact-content-length byte ranges its content occupies in
	// the image file - nil when Spec.ReportRanges was empty. A path's
	// ranges' lengths sum to exactly its written content length, even
	// though the FAT filesystem allocates it in whole clusters
	// underneath.
	FileRanges map[string][]ByteRange
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
// dataFS is the already-validated data filesystem (validateDataFilesystem's
// result): it decides whether the data partition gets FAT32's
// self-consistency trim (skipped for ext4, which has no such go-diskfs
// defect) or a minimum-size check against diskfmt.EXT4GoldenData.MinBytes()
// (skipped for FAT32, which has no fixed golden-image floor).
func computeLayout(bootSizeBytes, dataSizeBytes int64, dataFS diskfmt.FS) (layout, error) {
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

	if dataFS == diskfmt.EXT4 {
		minBytes := diskfmt.EXT4GoldenData.MinBytes()
		if trimmed := sizeInLBAs * sectorSizeBytes; trimmed < minBytes {
			return layout{}, fmt.Errorf(
				"data partition size %d bytes (%.2f MiB) is smaller than the smallest ext4 volume GoSD can write (%d bytes / %.2f MiB): %s; pass a larger --data-size, or build with --data-filesystem=fat32",
				trimmed, float64(trimmed)/(1<<20), minBytes, float64(minBytes)/(1<<20), diskfmt.EXT4GoldenData.SizeLimitReason())
		}
	} else {
		// Same trim as the boot partition above - a go-diskfs FAT32
		// sizing workaround with no ext4 equivalent (WriteEXT4 never
		// touches go-diskfs at all).
		sizeInLBAs = diskfmt.LargestSelfConsistentFAT32Bytes(sizeInLBAs*sectorSizeBytes) / sectorSizeBytes
	}

	lay.hasDataPartition = true
	lay.dataPartitionSizeInLBAs = uint32(sizeInLBAs)
	lay.totalSizeBytes = dataOffsetBytes + sizeInLBAs*sectorSizeBytes
	return lay, nil
}

// Write assembles a flashable MBR + FAT32 .img file at imgPath from spec. It
// is pure Go and requires no root privileges.
func Write(imgPath string, spec Spec) (report WriteReport, err error) {
	if err := validateReportRanges(spec.ReportRanges, spec.BootFiles); err != nil {
		return WriteReport{}, err
	}

	dataFS, err := validateDataFilesystem(spec.DataFilesystem)
	if err != nil {
		return WriteReport{}, err
	}

	if err := checkLabels(spec); err != nil {
		return WriteReport{}, err
	}

	lay, err := computeLayout(spec.BootSizeBytes, spec.DataSizeBytes, dataFS)
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
		dataPartitionType := mbr.Fat32LBA
		if dataFS == diskfmt.EXT4 {
			dataPartitionType = mbr.Linux
		}
		partitions = append(partitions, &mbr.Partition{
			Index: dataPartitionIndex,
			Type:  dataPartitionType,
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
		VolumeLabel: spec.BootLabel,
	})
	if err != nil {
		return WriteReport{}, fmt.Errorf("formatting the %s FAT32 boot partition failed: %w", spec.BootLabel, err)
	}

	fileSizes, payloadBytes, err := writeBootFiles(fs, spec.BootFiles)
	if err != nil {
		return WriteReport{}, wrapBootPartitionFullError(err, lay.bootPartitionSizeBytes)
	}

	var fileRanges map[string][]ByteRange
	if len(spec.ReportRanges) > 0 {
		// Collected from the same live fat32 handle the files were just
		// written through, so the ranges describe exactly what
		// writeBootFiles put down - no re-open/re-parse of the image.
		fileRanges, err = collectFileRanges(fs, spec.ReportRanges, fileSizes, lay)
		if err != nil {
			return WriteReport{}, err
		}
	}

	if lay.hasDataPartition {
		if dataFS == diskfmt.EXT4 {
			if err := writeEXT4DataPartition(d, lay, spec.DataLabel); err != nil {
				return WriteReport{}, err
			}
		} else if _, err := d.CreateFilesystem(disk.FilesystemSpec{
			Partition:   dataPartitionIndex,
			FSType:      filesystem.TypeFat32,
			VolumeLabel: spec.DataLabel,
		}); err != nil {
			return WriteReport{}, fmt.Errorf("formatting the %s FAT32 data partition failed: %w", spec.DataLabel, err)
		}
	}

	if err := applyRawWrites(d, spec.RawWrites, lay); err != nil {
		return WriteReport{}, err
	}

	return WriteReport{
		BootPartitionSizeBytes:    lay.bootPartitionSizeBytes,
		BootPartitionPayloadBytes: payloadBytes,
		FileRanges:                fileRanges,
	}, nil
}

// checkLabels rejects a Spec whose applicable volume labels can't be written
// as given, before diskfs.Create makes any image file exist. Only emptiness
// and FAT's 11-byte cap are checked here, and only as a guard against a
// programming error: go-diskfs formats a label through a "%-11.11s" verb,
// which silently truncates an over-long one - so an image would ship
// labelled something other than what its own config.json tells gosd-init to
// look for, and the device would reformat its data partition on the next
// boot. Every other label rule (charset, spaces, FAT's 8th-byte hazard)
// belongs to internal/blockmount.ValidateLabel, which the gosd CLI applies
// to --label-prefix at the boundary where a bad value is a person's typo -
// that function, not this one, is the full rule set.
//
// Spec.DataLabel is only checked when there is a partition 2 to label.
func checkLabels(spec Spec) error {
	if err := checkLabel("Spec.BootLabel", spec.BootLabel); err != nil {
		return err
	}
	if spec.DataSizeBytes == 0 {
		return nil
	}
	return checkLabel("Spec.DataLabel", spec.DataLabel)
}

// checkLabel is checkLabels' per-field check; field names the Spec field in
// the error, since both failures are a caller's bug to fix in code.
func checkLabel(field, label string) error {
	switch {
	case label == "":
		return fmt.Errorf("%s is empty; every image is built with an explicit volume label pair (see internal/naming.LabelsFor, and internal/blockmount.ValidateLabel for the rules the gosd CLI checks them against)", field)
	case len(label) > maxLabelLen:
		return fmt.Errorf("%s %q is %d bytes; a volume label is at most %d (see internal/blockmount.ValidateLabel for the full rule set)", field, label, len(label), maxLabelLen)
	}
	return nil
}

// validateDataFilesystem resolves Spec.DataFilesystem's zero value to
// diskfmt.FAT32 (today's default) and rejects anything other than FAT32 or
// EXT4 - diskfmt.ExFAT is a real diskfmt filesystem but is not yet wired up
// here, and any other value is simply not a filesystem GoSD knows.
func validateDataFilesystem(fs diskfmt.FS) (diskfmt.FS, error) {
	switch fs {
	case "":
		return diskfmt.FAT32, nil
	case diskfmt.FAT32, diskfmt.EXT4:
		return fs, nil
	default:
		return "", fmt.Errorf("Spec.DataFilesystem %q is not supported; only diskfmt.FAT32 (or the zero value) and diskfmt.EXT4 are accepted here (exFAT is not yet supported for the data partition)", string(fs))
	}
}

// validateReportRanges checks that every request names a key of bootFiles and
// that no path is asked for twice (WriteReport.FileRanges is keyed by path, so
// a second request would silently replace the first), before any image bytes
// exist (computeLayout, diskfs.Create, ...), so a typo'd --placeholder path
// fails cheaply instead of after a full build.
func validateReportRanges(reportRanges []string, bootFiles map[string]io.Reader) error {
	seen := make(map[string]bool, len(reportRanges))
	for _, p := range reportRanges {
		if _, ok := bootFiles[p]; !ok {
			return fmt.Errorf("Spec.ReportRanges names %q, which is not a Spec.BootFiles key; report only paths the boot files actually contain", p)
		}
		if seen[p] {
			return fmt.Errorf("Spec.ReportRanges names %q twice; WriteReport.FileRanges is keyed by path, so ask for each file once", p)
		}
		seen[p] = true
	}
	return nil
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
// any parent directories the path requires, and returns each path's written
// byte count (sizes - used to clip Spec.ReportRanges to exact content
// length in collectFileRanges) along with their total
// (WriteReport.BootPartitionPayloadBytes).
func writeBootFiles(fs filesystem.FileSystem, files map[string]io.Reader) (sizes map[string]int64, total int64, err error) {
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	sizes = make(map[string]int64, len(files))
	for _, p := range paths {
		if p == "" {
			return sizes, total, errors.New("boot file path must not be empty")
		}

		if dir := path.Dir(p); dir != "." {
			if err := fs.Mkdir(dir); err != nil {
				return sizes, total, fmt.Errorf("creating boot partition directory %q failed: %w", dir, err)
			}
		}

		f, err := fs.OpenFile(p, os.O_CREATE|os.O_RDWR)
		if err != nil {
			return sizes, total, fmt.Errorf("creating boot partition file %q failed: %w", p, err)
		}
		n, err := io.Copy(f, files[p])
		sizes[p] = n
		total += n
		if err != nil {
			_ = f.Close()
			return sizes, total, fmt.Errorf("writing boot partition file %q failed: %w", p, err)
		}
		if err := f.Close(); err != nil {
			return sizes, total, fmt.Errorf("closing boot partition file %q failed: %w", p, err)
		}
	}

	return sizes, total, nil
}

// fileRanger is implemented by go-diskfs's *fat12.File - the concrete type
// fs.OpenFile returns from the fat32.FileSystem Write formats, since
// fat32.FileSystem embeds *fat12.FileSystem (verified against go-diskfs
// v1.9.3). GetDiskRanges returns coalesced, whole-cluster,
// partition-relative byte ranges.
type fileRanger interface {
	GetDiskRanges() ([]fat12.DiskRange, error)
}

// collectFileRanges opens each of paths read-only on fs and returns its
// absolute, exact-content-length byte ranges within the image (for
// WriteReport.FileRanges). sizes holds each path's written byte count, from
// writeBootFiles; lay describes the partition layout every range must fall
// inside.
func collectFileRanges(fs filesystem.FileSystem, paths []string, sizes map[string]int64, lay layout) (map[string][]ByteRange, error) {
	result := make(map[string][]ByteRange, len(paths))
	for _, p := range paths {
		f, err := fs.OpenFile(p, 0)
		if err != nil {
			return nil, fmt.Errorf("reopening boot file %q to report its disk ranges failed: %w", p, err)
		}

		ranger, ok := f.(fileRanger)
		if !ok {
			_ = f.Close()
			return nil, fmt.Errorf("boot file %q opened as %T, which has no GetDiskRanges method; this should be impossible with go-diskfs's FAT32 filesystem", p, f)
		}

		diskRanges, err := ranger.GetDiskRanges()
		_ = f.Close()
		if err != nil {
			return nil, fmt.Errorf("getting disk ranges for boot file %q failed: %w", p, err)
		}

		ranges, err := absoluteContentRanges(diskRanges, sizes[p], lay)
		if err != nil {
			return nil, fmt.Errorf("boot file %q: %w", p, err)
		}
		result[p] = ranges
	}
	return result, nil
}

// absoluteContentRanges converts diskRanges - partition-relative, whole
// clusters, from (*fat12.File).GetDiskRanges - into absolute ByteRanges
// clipped to exactly contentSize bytes: the injection manifest contract
// needs exact content ranges, not the whole clusters FAT32 rounds a file's
// storage up to. Every range must lie entirely inside partition 1
// ([bootPartitionOffsetBytes, +lay.bootPartitionSizeBytes)), and the
// ranges' total length must be at least contentSize (go-diskfs's cluster
// allocation always covers at least the file's written size).
func absoluteContentRanges(diskRanges []fat12.DiskRange, contentSize int64, lay layout) ([]ByteRange, error) {
	bootPartitionEndBytes := bootPartitionOffsetBytes + lay.bootPartitionSizeBytes

	absolute := make([]ByteRange, len(diskRanges))
	var total int64
	for i, dr := range diskRanges {
		offset := bootPartitionOffsetBytes + int64(dr.Offset)
		length := int64(dr.Length)
		if offset < bootPartitionOffsetBytes || offset+length > bootPartitionEndBytes {
			return nil, fmt.Errorf("disk range [%d, %d) falls outside the boot partition [%d, %d)",
				offset, offset+length, bootPartitionOffsetBytes, bootPartitionEndBytes)
		}
		absolute[i] = ByteRange{OffsetBytes: offset, LengthBytes: length}
		total += length
	}
	if total < contentSize {
		return nil, fmt.Errorf("disk ranges total %d bytes, less than the file's %d written bytes", total, contentSize)
	}

	clipped := make([]ByteRange, 0, len(absolute))
	var seen int64
	for _, r := range absolute {
		if seen >= contentSize {
			break
		}
		if remaining := contentSize - seen; r.LengthBytes > remaining {
			r.LengthBytes = remaining
		}
		clipped = append(clipped, r)
		seen += r.LengthBytes
	}
	return clipped, nil
}

// writeEXT4DataPartition writes the data partition's ext4 golden filesystem
// directly to the image's backing file, bypassing go-diskfs entirely (unlike
// the FAT32 branch's d.CreateFilesystem): diskfmt.WriteEXT4 needs only an
// io.WriterAt, which offsetPartitionWriter supplies confined to the data
// partition's own byte range within the image.
func writeEXT4DataPartition(d *disk.Disk, lay layout, label string) error {
	w, err := d.Backend.Writable()
	if err != nil {
		return fmt.Errorf("opening the image for the %s ext4 data partition failed: %w", label, err)
	}

	sizeBytes := int64(lay.dataPartitionSizeInLBAs) * sectorSizeBytes
	shifted := offsetPartitionWriter{w: w, base: lay.dataPartitionOffsetBytes, limit: sizeBytes}
	if err := diskfmt.WriteEXT4(diskfmt.EXT4GoldenData, shifted, sizeBytes, label); err != nil {
		return fmt.Errorf("writing the %s ext4 data partition failed: %w", label, err)
	}
	return nil
}

// offsetPartitionWriter confines an io.WriterAt to the byte range
// [base, base+limit), rewriting every WriteAt as relative to that range's
// start: diskfmt.WriteEXT4 addresses the region it is given as if it began
// at byte 0, so this is what lets it write a self-contained ext4 filesystem
// into the data partition without ever knowing that partition's offset
// inside the image, and without any possibility of overrunning into the
// boot partition or past the end of the image.
type offsetPartitionWriter struct {
	w     io.WriterAt
	base  int64
	limit int64
}

func (o offsetPartitionWriter) WriteAt(p []byte, off int64) (int, error) {
	if off < 0 {
		return 0, fmt.Errorf("write at negative offset %d within the data partition", off)
	}
	if end := off + int64(len(p)); end > o.limit {
		return 0, fmt.Errorf("write of %d bytes at offset %d would end at byte %d, past the end of the %d-byte data partition", len(p), off, end, o.limit)
	}
	return o.w.WriteAt(p, o.base+off)
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
		return fmt.Errorf("%w: write at offset %d (%d bytes) overlaps partition 2, the data partition (bytes %d-%d); "+
			"raw writes must stay within the unpartitioned gap (bytes %d-%d)",
			ErrRawWriteOverlap, offset, length, lay.dataPartitionOffsetBytes, lay.totalSizeBytes,
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
