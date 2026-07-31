// Package dataexpand creates the GOSD-DATA partition on first boot for
// images built with --data-size=expand. Such an image ships with no
// partition 2 at all; this package grows the card into one by telling the
// running kernel about the partition, formatting it FAT32, and only then
// writing its MBR entry — all before the normal data-partition mount runs.
//
// Where that partition starts is read from the flashed MBR — the sector
// after partition 1 — never assumed: the boot volume's size is chosen per
// app at build time, so only the table on this card knows the layout it was
// flashed with.
//
// The MBR entry is the commit record of a completed first boot, written
// only over a filesystem proven finished by EstablishedMarker. Three states
// therefore cover every boot: no entry means first-boot work is (re)done
// from scratch — power loss anywhere mid-creation lands back here, as does
// reflashing the card (which rewrites the MBR without a partition 2 while
// leaving the data region's bytes untouched), so what already occupies the
// partition is inspected and a marked GOSD-DATA filesystem adopted rather
// than reformatted; an entry over a mountable GOSD-DATA filesystem means
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
	"math"
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

	// bootPartitionStartLBA mirrors internal/image's locked start for
	// partition 1: 16MiB, leaving room for a board's raw bootloader ahead
	// of it. Its *size* is per-app, so where partition 2 goes is derived
	// from the flashed table (see dataStartLBA), never mirrored here.
	bootPartitionStartLBA = 16 * 1024 * 1024 / sectorSize

	// fatPartitionType is MBR type 0x0C (FAT32, LBA addressing), the same
	// type internal/image writes for both of its partitions.
	fatPartitionType = 0x0C

	// Label is the volume label the created filesystem carries, identical
	// to the GOSD-DATA partition a fixed --data-size build ships.
	Label = "GOSD-DATA"

	// EstablishedMarker is an empty file this package writes into the root
	// of a filesystem it has just formatted AND flushed, and looks for
	// before adopting one it finds (see survivorPresent). It exists because
	// the volume label is not evidence of a finished format: go-diskfs
	// writes the boot sector, FATs, root directory and finally the label
	// with no sync between them, so a power cut mid-format can leave a
	// volume that inspects as FAT32 labelled GOSD-DATA over incomplete FAT
	// tables. Adopting that debris would commit an MBR entry over a broken
	// filesystem forever; the marker, written only after the format's sync
	// barrier, means "everything before that barrier reached the medium".
	//
	// It is reserved: apps must leave it alone. Deleting it costs nothing
	// on an established partition (verifyEstablished never looks for it,
	// precisely so an app's stray delete can't be read as corruption) —
	// only a partition being re-adopted after a reflash needs it.
	//
	// Not a dotfile, unlike the mount layer's /data/.gosd-data: go-diskfs
	// cannot create a leading-dot name it can later find (see
	// diskfmt.CreateEmptyFile).
	//
	// The GOSD-DATA a fixed --data-size image embeds carries no marker, by
	// choice: that partition ships with an MBR entry, so it is never a
	// candidate for adoption, and a card reflashed from a fixed-size image
	// to an expand one is reformatted exactly as it was before this marker
	// existed.
	EstablishedMarker = "gosd-data-established"

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
	// CreateMarker writes EstablishedMarker into the root of the
	// filesystem on the partition node, without mounting it.
	CreateMarker func(partitionDevice string) error
	// MarkerExists reports whether that marker is there. An error means the
	// filesystem's root directory could not be read at all.
	MarkerExists func(partitionDevice string) (bool, error)
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
	startLBA := dataStartLBA(mbr)
	sizeSectors, reason, note := partitionSectors(deviceBytes, startLBA)
	if sizeSectors == 0 {
		deps.Log("not creating a data partition: %s", reason)
		return nil
	}
	if note != "" {
		deps.Log("%s", note)
	}

	// Creation order is the crash-safety contract, and every step's position
	// in it is load-bearing. The kernel learns of the partition (in-memory
	// state only). The filesystem is written, flushed, marked established,
	// and flushed again — the second flush is what makes the marker's
	// presence imply, by program order, that the format before it also
	// reached the medium. Only then does the MBR entry — the on-disk commit
	// record — go in. Power loss before it leaves no entry, so the next boot
	// starts over, and finds either a marked filesystem (adopt) or debris
	// (reformat); either way it converges. An entry therefore always means a
	// marker-verified filesystem, which is what lets verifyEstablished treat
	// anything else under one as real corruption.
	//
	// Adoption reuses this boot's geometry for a filesystem an earlier boot
	// sized: same derivation, same device, so the same offset and length.
	if err := deps.AddKernelPartition(opts.Device, dataPartitionNumber,
		startLBA*sectorSize, sizeSectors*sectorSize); err != nil {
		return err
	}
	if err := waitForNode(deps, opts.PartitionDevice, opts.NodeTimeout); err != nil {
		return err
	}
	adopt, err := survivorPresent(deps, opts.PartitionDevice)
	if err != nil {
		return err
	}
	if !adopt {
		deps.Log("formatting %s as %s (%s) — one-time first-boot setup", opts.PartitionDevice, Label, sizeString(sizeSectors*sectorSize))
		if err := deps.FormatFAT32(opts.PartitionDevice, Label); err != nil {
			return err
		}
		if err := deps.SyncDevice(opts.PartitionDevice); err != nil {
			return fmt.Errorf("flushing the new filesystem to %s: %w", opts.PartitionDevice, err)
		}
		if err := deps.CreateMarker(opts.PartitionDevice); err != nil {
			return fmt.Errorf("recording the completed format on %s: %w", opts.PartitionDevice, err)
		}
		if err := deps.SyncDevice(opts.PartitionDevice); err != nil {
			return fmt.Errorf("flushing the completed-format marker to %s: %w", opts.PartitionDevice, err)
		}
	}

	writeDataEntry(mbr, uint32(startLBA), uint32(sizeSectors))
	if err := deps.WriteMBR(opts.Device, mbr); err != nil {
		return fmt.Errorf("writing the new partition table to %s: %w", opts.Device, err)
	}
	if adopt {
		deps.Log("data partition re-adopted, its contents intact")
	} else {
		deps.Log("data partition created, filling the card")
	}
	return nil
}

// survivorPresent reports whether the partition already holds a GOSD-DATA
// filesystem worth keeping — the state a plain reflash leaves behind, since
// writing an image rewrites the MBR (dropping partition 2's entry) without
// touching the bytes beyond the boot partition. Adoption needs the derived
// offset, FAT32, the exact label (the gate blockmount applies to every other
// mount decision) AND EstablishedMarker, which is the only proof the format
// that wrote that label ever finished. Anything else — blank space, a
// foreign volume, the unrecognisable middle of a filesystem whose start a
// differently sized boot volume overwrote, the debris of an interrupted
// format — is formatted fresh, as it always was.
//
// A partition that fails to identify at all is not "anything else": nothing
// may be formatted over contents that could not be seen, so this boot gives
// up (leaving /data read-only) and the next one tries again. A partition
// that identifies as GOSD-DATA but whose root directory then fails to read
// IS: that combination is a hallmark of a half-written filesystem, and
// treating it as unadoptable is what keeps an interrupted format
// self-healing rather than wedging the device forever.
func survivorPresent(deps Deps, partitionDevice string) (bool, error) {
	contents, err := deps.Inspect(partitionDevice)
	if err != nil {
		return false, fmt.Errorf("reading %s to check whether it already holds %s data: %w", partitionDevice, Label, err)
	}
	if contents.FS != diskfmt.FAT32 || contents.Label != Label {
		return false, nil
	}

	marked, err := deps.MarkerExists(partitionDevice)
	if err != nil {
		deps.Log("%s looks like %s but its root directory could not be read (%v); treating it as the debris of an interrupted format", partitionDevice, Label, err)
		return false, nil
	}
	if !marked {
		deps.Log("%s looks like %s but carries no format-completion marker; treating it as the debris of an interrupted format", partitionDevice, Label)
		return false, nil
	}
	return true, nil
}

// dataStartLBA is the sector the data partition begins at: the first one past
// partition 1, read from the table the image writer put on this card and the
// flash faithfully reproduced. It cannot be a constant — the boot volume's
// size is chosen per app at build time (gosd build --boot-size) — and
// checkGosdMBR has already rejected a partition 1 that ends nowhere sane.
func dataStartLBA(mbr []byte) int64 {
	_, startLBA, sizeLBA := readEntry(mbr, bootPartitionNumber)
	return int64(startLBA) + int64(sizeLBA)
}

// verifyEstablished handles a boot where the MBR already lists partition 2.
// The entry is only ever written over a filesystem this package has proven
// finished — formatted, flushed and marked here, or adopted only once
// EstablishedMarker showed an earlier boot's format finished (see Run) — and
// flashing an image rewrites the MBR without an entry, so an entry means an
// established partition that may hold app data. Either it still carries its
// GOSD-DATA filesystem (the every-later-boot happy path: nothing to do), or
// something has gone genuinely wrong with data possibly at stake — reported
// as ErrDataCorrupt, never repaired here.
//
// The marker deliberately plays no part in this check: /data belongs to the
// app from here on, and an app that tidies away a file it did not expect
// must not thereby turn its own working partition into a corruption halt.
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
// on a device of deviceBytes: the space from startLBA to the end of the
// device, aligned down to alignBytes and capped at maxPartitionBytes. A zero
// return means "create nothing", with reason saying why; note, when
// non-empty, is worth logging even though creation proceeds.
func partitionSectors(deviceBytes, startLBA int64) (sectors int64, reason, note string) {
	free := deviceBytes/sectorSize - startLBA
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
// signature, and partition 1 of the locked type at the locked offset, ending
// somewhere an MBR can address (its size is per-app, but the sector after it
// is where everything below puts the data partition). Anything else means
// this is not the card layout this package understands, and nothing is
// touched.
func checkGosdMBR(mbr []byte) error {
	if len(mbr) != mbrSize {
		return fmt.Errorf("read %d bytes of partition table, want %d", len(mbr), mbrSize)
	}
	if mbr[signatureOffset] != 0x55 || mbr[signatureOffset+1] != 0xAA {
		return fmt.Errorf("no MBR boot signature")
	}
	partType, startLBA, sizeLBA := readEntry(mbr, bootPartitionNumber)
	if partType != fatPartitionType || startLBA != bootPartitionStartLBA {
		return fmt.Errorf("partition 1 is type %#02x at sector %d, want type %#02x at sector %d",
			partType, startLBA, fatPartitionType, bootPartitionStartLBA)
	}
	if end := int64(startLBA) + int64(sizeLBA); sizeLBA == 0 || end > math.MaxUint32 {
		return fmt.Errorf("partition 1 is %d sectors long, which puts its end at sector %d — not a usable start for the data partition",
			sizeLBA, end)
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
