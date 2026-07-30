// Package dataexpand creates the GOSD-DATA partition on first boot for
// images built with --data-size=expand. Such an image ships with no
// partition 2 at all (staying 272MiB); this package grows the card into one
// by telling the running kernel about the partition, formatting it FAT32,
// and only then writing its MBR entry — all before the normal
// data-partition mount runs.
//
// The MBR entry is the commit record of a completed first boot, written
// only after the formatted filesystem is durable on the card. Three states
// therefore cover every boot: no entry means first-boot work is (re)done
// from scratch — power loss anywhere mid-creation lands back here, with no
// user data at stake; an entry over a mountable GOSD-DATA filesystem means
// everything already happened, and nothing is touched; an entry over
// anything else means an established partition — possibly carrying app
// data — has been corrupted, reported as ErrDataCorrupt so the caller can
// halt the device rather than let anything destroy what might be
// recoverable. Every other failure is ordinary and non-fatal: the caller
// logs it and falls back to the read-only /data placeholder for this boot.
package dataexpand

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jphastings/gosd/internal/diskfmt"
)

// ErrDataCorrupt reports the one state Run refuses to repair: the partition
// table says the data partition is established (its entry is only ever
// written after a completed, synced format), but what it holds is not the
// GOSD-DATA filesystem that format left. App data may be at stake, so
// callers are expected to halt the device rather than mount around it —
// see boot.Run's handling.
var ErrDataCorrupt = errors.New("the data partition is corrupt")

const (
	sectorSize = 512
	mbrSize    = 512

	// bootPartitionStartLBA and dataPartitionStartLBA mirror
	// internal/image's locked on-disk layout: partition 1 at 16MiB, and
	// partition 2 directly after its 256MiB, at 272MiB.
	bootPartitionStartLBA = 16 * 1024 * 1024 / sectorSize
	dataPartitionStartLBA = (16 + 256) * 1024 * 1024 / sectorSize

	// fatPartitionType is MBR type 0x0C (FAT32, LBA addressing), the same
	// type internal/image writes for both of its partitions.
	fatPartitionType = 0x0C

	// Label is the volume label the created filesystem carries, identical
	// to the GOSD-DATA partition a fixed --data-size build ships.
	Label = "GOSD-DATA"

	// minPartitionBytes is the smallest data partition worth creating.
	// FAT32 needs ~33MiB just to exist; a card leaving less free space
	// than this is essentially no bigger than the image, and gets no data
	// partition — exactly like a --data-size=0 image.
	minPartitionBytes = 64 * 1024 * 1024

	// maxPartitionBytes caps the created partition at 256GiB: go-diskfs's
	// FAT32 formatter computes its sectors-per-FAT count through a uint16,
	// which silently truncates — corrupting the filesystem — for volumes
	// past roughly this size. 256GiB is exactly within the safe range.
	// Lifting the cap is tracked in bean gosd-8kdm.
	maxPartitionBytes = 256 * 1024 * 1024 * 1024

	// alignBytes aligns the partition's size down to 4MiB, a comfortable
	// multiple of every common SD-card erase-block size.
	alignBytes = 4 * 1024 * 1024

	partitionEntriesOffset = 446
	partitionEntrySize     = 16
	bootPartitionNumber    = 1
	dataPartitionNumber    = 2
	signatureOffset        = 510
)

// Deps bundles every side-effecting dependency the expansion needs.
// Production wiring (NewDeps in platform_linux.go) supplies the real
// syscall-backed implementations; tests supply fakes.
type Deps struct {
	// ReadMBR reads the device's first sector: the 512-byte MBR.
	ReadMBR func(device string) ([]byte, error)
	// WriteMBR writes the 512-byte MBR back and syncs the device.
	WriteMBR func(device string, sector []byte) error
	// DeviceSizeBytes reports the whole device's size.
	DeviceSizeBytes func(device string) (int64, error)
	// AddKernelPartition registers partition partNo, spanning
	// [startBytes, startBytes+sizeBytes), with the running kernel so its
	// device node appears — without the whole-table reread that fails
	// while partition 1 is mounted as /boot.
	AddKernelPartition func(device string, partNo int, startBytes, sizeBytes int64) error
	// Inspect reports what filesystem (if any) a partition node carries.
	Inspect func(partitionDevice string) (diskfmt.Contents, error)
	// FormatFAT32 writes a FAT32 filesystem labelled label onto the
	// partition node.
	FormatFAT32 func(partitionDevice, label string) error
	// SyncDevice flushes a device node's dirty pages to the medium, making
	// a just-written format durable before the MBR entry commits to it.
	SyncDevice func(device string) error
	// PathExists reports whether a device node exists yet.
	PathExists func(path string) bool

	Sleep func(time.Duration)
	Now   func() time.Time
	Log   func(format string, args ...any)
}

// Options names the two device nodes one expansion acts on.
type Options struct {
	// Device is the whole-disk node the system booted from (e.g.
	// /dev/mmcblk0) — the only device ever considered for expansion.
	Device string
	// PartitionDevice is Device's partition-2 node (e.g. /dev/mmcblk0p2),
	// which appears once the kernel learns of the new partition.
	PartitionDevice string
	// NodeTimeout bounds how long Run waits for PartitionDevice to appear
	// after telling the kernel about the partition.
	NodeTimeout time.Duration
}

// Run performs one boot's worth of expansion work, deciding everything from
// the MBR as described in the package comment. It returns nil both when the
// partition is ready and when there is legitimately nothing to do (no room
// on the card); an error means /data will be read-only this boot, and is the
// caller's to log — never fatal.
func Run(deps Deps, opts Options) error {
	mbr, err := deps.ReadMBR(opts.Device)
	if err != nil {
		return fmt.Errorf("reading %s's partition table: %w", opts.Device, err)
	}
	if err := checkGosdMBR(mbr); err != nil {
		return fmt.Errorf("%s does not carry the expected GoSD partition table, leaving it untouched: %w", opts.Device, err)
	}

	if partType, _, _ := readEntry(mbr, dataPartitionNumber); partType != 0 {
		return verifyEstablished(deps, opts)
	}

	deviceBytes, err := deps.DeviceSizeBytes(opts.Device)
	if err != nil {
		return fmt.Errorf("finding %s's size: %w", opts.Device, err)
	}
	sizeSectors, reason, note := partitionSectors(deviceBytes)
	if sizeSectors == 0 {
		deps.Log("not creating a data partition: %s", reason)
		return nil
	}
	if note != "" {
		deps.Log("%s", note)
	}

	// Creation order is the crash-safety contract: the kernel learns of the
	// partition (in-memory state only), the filesystem is written and made
	// durable, and only then does the MBR entry — the on-disk commit record
	// — go in. Power loss anywhere before that final write leaves no entry,
	// so the next boot redoes everything from scratch; an entry therefore
	// always means a completed format, which is what lets verifyEstablished
	// treat anything else under one as real corruption.
	if err := deps.AddKernelPartition(opts.Device, dataPartitionNumber,
		dataPartitionStartLBA*sectorSize, sizeSectors*sectorSize); err != nil {
		return err
	}
	if err := waitForNode(deps, opts.PartitionDevice, opts.NodeTimeout); err != nil {
		return err
	}
	deps.Log("formatting %s as %s (%s) — one-time first-boot setup", opts.PartitionDevice, Label, sizeString(sizeSectors*sectorSize))
	if err := deps.FormatFAT32(opts.PartitionDevice, Label); err != nil {
		return err
	}
	if err := deps.SyncDevice(opts.PartitionDevice); err != nil {
		return fmt.Errorf("flushing the new filesystem to %s: %w", opts.PartitionDevice, err)
	}

	writeDataEntry(mbr, dataPartitionStartLBA, uint32(sizeSectors))
	if err := deps.WriteMBR(opts.Device, mbr); err != nil {
		return fmt.Errorf("writing the new partition table to %s: %w", opts.Device, err)
	}
	deps.Log("data partition created, filling the card")
	return nil
}

// verifyEstablished handles a boot where the MBR already lists partition 2.
// The entry is only ever written after a completed, synced format (see Run),
// and flashing an image rewrites the MBR without one, so an entry means an
// established partition that may hold app data. Either it still carries its
// GOSD-DATA filesystem (the every-later-boot happy path: nothing to do), or
// something has gone genuinely wrong with data possibly at stake — reported
// as ErrDataCorrupt, never repaired here.
func verifyEstablished(deps Deps, opts Options) error {
	if !deps.PathExists(opts.PartitionDevice) {
		return fmt.Errorf("%w: the partition table lists it, but its device node %s never appeared", ErrDataCorrupt, opts.PartitionDevice)
	}
	contents, err := deps.Inspect(opts.PartitionDevice)
	if err != nil {
		return fmt.Errorf("%w: reading %s failed: %v", ErrDataCorrupt, opts.PartitionDevice, err)
	}
	if contents.FS == diskfmt.FAT32 && contents.Label == Label {
		deps.Log("data partition already present on %s", opts.PartitionDevice)
		return nil
	}
	return fmt.Errorf("%w: %s holds %s where a FAT32 filesystem labelled %s should be", ErrDataCorrupt, opts.PartitionDevice, describeContents(contents), Label)
}

// describeContents names what Inspect found, for the corruption report a
// person will eventually read off the card.
func describeContents(c diskfmt.Contents) string {
	switch {
	case c.FS != "":
		return fmt.Sprintf("a %s filesystem labelled %q", c.FS, c.Label)
	case c.OtherFS != "":
		return "an unreadable " + c.OtherFS + " filesystem"
	case c.Blank:
		return "nothing (blank space)"
	default:
		return "unrecognisable content"
	}
}

// partitionSectors decides the size, in sectors, of the partition to create
// on a device of deviceBytes: the space after the boot partition, aligned
// down to alignBytes and capped at maxPartitionBytes. A zero return means
// "create nothing", with reason saying why; note, when non-empty, is worth
// logging even though creation proceeds.
func partitionSectors(deviceBytes int64) (sectors int64, reason, note string) {
	free := deviceBytes/sectorSize - dataPartitionStartLBA
	free -= free % (alignBytes / sectorSize)
	if free*sectorSize < minPartitionBytes {
		return 0, fmt.Sprintf("the card (%s) leaves less than %s beyond the image; treating it like --data-size=0",
			sizeString(deviceBytes), sizeString(minPartitionBytes)), ""
	}
	if free*sectorSize > maxPartitionBytes {
		unused := free*sectorSize - maxPartitionBytes
		return maxPartitionBytes / sectorSize, "",
			fmt.Sprintf("capping the data partition at %s (FAT32 formatter limit); %s of the card stays unused",
				sizeString(maxPartitionBytes), sizeString(unused))
	}
	return free, "", ""
}

// checkGosdMBR confirms sector 0 is the MBR a GoSD image ships: the boot
// signature, and partition 1 of the locked type at the locked offset.
// Anything else means this is not the card layout this package understands,
// and nothing is touched.
func checkGosdMBR(mbr []byte) error {
	if len(mbr) != mbrSize {
		return fmt.Errorf("read %d bytes of partition table, want %d", len(mbr), mbrSize)
	}
	if mbr[signatureOffset] != 0x55 || mbr[signatureOffset+1] != 0xAA {
		return fmt.Errorf("no MBR boot signature")
	}
	partType, startLBA, _ := readEntry(mbr, bootPartitionNumber)
	if partType != fatPartitionType || startLBA != bootPartitionStartLBA {
		return fmt.Errorf("partition 1 is type %#02x at sector %d, want type %#02x at sector %d",
			partType, startLBA, fatPartitionType, bootPartitionStartLBA)
	}
	return nil
}

// readEntry decodes MBR partition entry n (1-based). A zero partType means
// the slot is empty.
func readEntry(mbr []byte, n int) (partType byte, startLBA, sizeLBA uint32) {
	entry := mbr[partitionEntriesOffset+(n-1)*partitionEntrySize:]
	return entry[4], binary.LittleEndian.Uint32(entry[8:12]), binary.LittleEndian.Uint32(entry[12:16])
}

// writeDataEntry fills MBR partition entry 2 in place: not bootable, CHS
// fields set to the 0xFE/0xFF/0xFF "use LBA" marker every modern tool
// writes, type 0x0C, and the given LBA geometry.
func writeDataEntry(mbr []byte, startLBA, sizeLBA uint32) {
	entry := mbr[partitionEntriesOffset+(dataPartitionNumber-1)*partitionEntrySize:]
	entry[0] = 0x00                                 // not bootable
	entry[1], entry[2], entry[3] = 0xFE, 0xFF, 0xFF // CHS start: beyond CHS, use LBA
	entry[4] = fatPartitionType
	entry[5], entry[6], entry[7] = 0xFE, 0xFF, 0xFF // CHS end: same
	binary.LittleEndian.PutUint32(entry[8:12], startLBA)
	binary.LittleEndian.PutUint32(entry[12:16], sizeLBA)
}

// waitForNode polls for the partition's device node: devtmpfs creates it
// almost immediately after the kernel learns of the partition, but there is
// no udev to synchronize on, so poll briefly rather than assume.
func waitForNode(deps Deps, path string, timeout time.Duration) error {
	deadline := deps.Now().Add(timeout)
	for !deps.PathExists(path) {
		if !deps.Now().Before(deadline) {
			return fmt.Errorf("%s did not appear within %s of the kernel learning about the partition", path, timeout)
		}
		deps.Sleep(50 * time.Millisecond)
	}
	return nil
}

// DataPartitionFor derives, from the partition-1 node the boot partition
// mounted from, the whole-disk device to expand and the partition-2 node the
// data partition will use: /dev/mmcblk0p1 yields (/dev/mmcblk0,
// /dev/mmcblk0p2); /dev/vda1 yields (/dev/vda, /dev/vda2). The kernel's
// naming rule is the reverse of boot.FilterBootDevices' onDisk: disks whose
// name ends in a digit take a "p" separator before the partition number. ok
// is false for anything that is not a first-partition node.
func DataPartitionFor(bootPartition string) (device, partition2 string, ok bool) {
	base, found := strings.CutSuffix(bootPartition, "1")
	if !found || base == "" {
		return "", "", false
	}
	device = base
	if disk, hasP := strings.CutSuffix(base, "p"); hasP && disk != "" && disk[len(disk)-1] >= '0' && disk[len(disk)-1] <= '9' {
		device = disk
	}
	return device, base + "2", true
}

// sizeString renders a byte count the way a person flashing a card thinks of
// it: whole-number MiB below 1GiB, one-decimal GiB above.
func sizeString(bytes int64) string {
	const gib = 1024 * 1024 * 1024
	if bytes >= gib {
		return fmt.Sprintf("%.1fGiB", float64(bytes)/gib)
	}
	return fmt.Sprintf("%dMiB", bytes/(1024*1024))
}
