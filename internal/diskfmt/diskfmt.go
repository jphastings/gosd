// Package diskfmt inspects and formats whole block devices on-device, in pure
// Go (CGO_ENABLED=0), with no external mkfs and no root beyond write access to
// the device node. It backs the public emmc and disk packages.
//
// FAT32 comes from github.com/diskfs/go-diskfs — already used to build image
// files in internal/image — which can target a real block device:
// CreateFilesystem with Partition 0 writes a whole-device FAT32 with no
// partition table (which also avoids the BLKRRPART reread that needs
// privileges). Devices are opened and sized here (see openDisk) rather than
// by diskfs.Open, whose BLKGETSIZE64 sizing is broken on 32-bit ARM.
// go-diskfs has no exFAT support at all, so exFAT is read and written here
// directly against the Microsoft exFAT specification — see exfat.go and
// exfatformat.go.
package diskfmt

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/diskfs/go-diskfs/backend/file"
	"github.com/diskfs/go-diskfs/disk"
	"github.com/diskfs/go-diskfs/filesystem"
)

// sectorSizeBytes is the logical sector size every filesystem here is laid
// out with. SD cards, eMMC and USB-attached media universally present
// 512-byte logical sectors, and FormatExFAT already fixes its own layout at
// 512 (see exFATFormatSectorShift).
const sectorSizeBytes = 512

// FS names a filesystem diskfmt can identify, create and mount.
type FS string

const (
	// FAT32 is the universal default: every host mounts it, and it needs no
	// kernel support beyond the CONFIG_VFAT_FS every GoSD board already has.
	FAT32 FS = "fat32"
	// ExFAT lifts FAT32's 4 GiB per-file ceiling, at the cost of needing
	// CONFIG_EXFAT_FS in the board's kernel.
	ExFAT FS = "exfat"
	// EXT4 is journaled and crash-safe, at the cost of not being readable by
	// every host OS out of the box; see internal/diskfmt/ext4golden for how
	// GoSD creates one (a checked-in golden image, never mkfs.ext4).
	EXT4 FS = "ext4"
	// FAT16 and FAT12 are narrower FAT widths GoSD never creates (Format has
	// no case for either) but Inspect can find on a device it did not write
	// — e.g. a stick someone else formatted. Reported distinctly so a
	// refusal names what is actually there instead of misreporting it as
	// FAT32 (gosd-8rw2); both mount the same way FAT32 does.
	FAT16 FS = "fat16"
	FAT12 FS = "fat12"
)

// MountType is the name mount(2) knows this filesystem by, which is not always
// the name people call it: every FAT width is Linux's "vfat". Empty for an FS
// value GoSD does not handle.
func (f FS) MountType() string {
	switch f {
	case FAT32, FAT16, FAT12:
		return "vfat"
	case ExFAT:
		return "exfat"
	case EXT4:
		return "ext4"
	default:
		return ""
	}
}

// String names the filesystem the way an app author would write it.
func (f FS) String() string {
	switch f {
	case FAT32:
		return "FAT32"
	case FAT16:
		return "FAT16"
	case FAT12:
		return "FAT12"
	case ExFAT:
		return "exFAT"
	case EXT4:
		return "ext4"
	default:
		return string(f)
	}
}

// blankProbeBytes is how much of the start of a device Inspect reads to decide
// whether it is "blank". It comfortably spans an MBR (sector 0), a GPT header
// and its primary entry array (sectors 1-33), and a FAT or exFAT boot region,
// so any existing partition table or filesystem leaves a non-zero byte in it.
const blankProbeBytes = 1 << 20 // 1 MiB

// Contents describes what already occupies a block device, which is all the
// mount-only / format / refuse decision depends on.
type Contents struct {
	// FS names the filesystem the device carries, empty when it carries none
	// GoSD can mount.
	FS FS

	// Label is that filesystem's volume label, trimmed of its padding.
	// Meaningful only when FS is set.
	Label string

	// UUID is that filesystem's volume UUID, in canonical 8-4-4-4-12 hex.
	// Only ext4 carries one today; empty for FAT32 and exFAT. Meaningful
	// only when FS is set.
	UUID string

	// Blank is true when the device has no readable filesystem and its
	// leading region is entirely zero — nothing to destroy, so it is safe to
	// format even without an explicit destructive opt-in. Meaningful only
	// when FS is empty.
	Blank bool

	// OtherFS names a filesystem recognised on the device but not readable,
	// e.g. an exFAT volume whose geometry does not parse, so a refusal to
	// overwrite it can say what it is. Meaningful only when FS is empty.
	OtherFS string
}

// Inspect reports what occupies the block device (or image file) at devicePath.
// It never modifies the device.
func Inspect(devicePath string) (Contents, error) {
	head, err := readLeadingRegion(devicePath)
	if err != nil {
		return Contents{}, err
	}

	// exFAT is probed first because a real exFAT boot sector carries the same
	// 0xAA55 signature at offset 510 that a FAT probe looks for, so letting
	// go-diskfs guess first risks it claiming an exFAT volume as FAT.
	if isExFAT(head) {
		return inspectExFAT(devicePath), nil
	}

	// ext4's superblock lives at a fixed offset with its own magic, so there
	// is no ambiguity with FAT/exFAT's boot-sector signatures to resolve by
	// ordering; unlike inspectExFAT, a failure here is returned rather than
	// swallowed — see inspectEXT4.
	if isEXT4(head) {
		return inspectEXT4(devicePath, head)
	}

	if fat, ok, err := inspectFAT(devicePath); err != nil {
		return Contents{}, err
	} else if ok {
		return fat, nil
	}
	return Contents{Blank: isAllZero(head)}, nil
}

// isExFAT reports whether a device's leading bytes are an exFAT boot sector.
// exFAT writes "EXFAT   " at offset 3, where FAT and NTFS put their own OEM
// name.
func isExFAT(head []byte) bool {
	const offset = 3
	return len(head) >= offset+len(exFATMagic) && bytes.Equal(head[offset:offset+len(exFATMagic)], exFATMagic)
}

// inspectExFAT reads the volume label of a device whose boot sector announces
// exFAT. A volume that announces itself but whose geometry does not parse is
// named rather than claimed: Contents.OtherFS makes a refusal specific without
// ever promising the volume is mountable.
func inspectExFAT(devicePath string) Contents {
	label, err := readExFATLabel(devicePath)
	if err != nil {
		return Contents{OtherFS: ExFAT.String()}
	}
	return Contents{FS: ExFAT, Label: label}
}

// inspectFAT reports whether devicePath holds a FAT filesystem and, if so,
// its width-specific FS and label. ok is false (with a nil error) when the
// device simply isn't FAT; a non-nil error means the device could not be
// read at all.
func inspectFAT(devicePath string) (contents Contents, ok bool, err error) {
	d, err := openDisk(devicePath, true)
	if err != nil {
		return Contents{}, false, fmt.Errorf("opening %s to inspect it failed: %w", devicePath, err)
	}
	defer func() { _ = d.Close() }()

	// GetFilesystem probes FAT32/16/12 (then other, non-FAT types); an error
	// or a non-FAT result both mean "not a FAT we recognise".
	fs, err := d.GetFilesystem(0)
	if err != nil {
		return Contents{}, false, nil
	}
	fatFS, ok := fatWidth(fs.Type())
	if !ok {
		return Contents{}, false, nil
	}
	return Contents{FS: fatFS, Label: trimLabel(fs.Label())}, true, nil
}

// fatWidth maps a go-diskfs FAT type to the FS value Inspect reports it as,
// so a refusal names the width that is actually on the device (gosd-8rw2)
// instead of Format's own FAT32-only vocabulary.
func fatWidth(t filesystem.Type) (FS, bool) {
	switch t {
	case filesystem.TypeFat32:
		return FAT32, true
	case filesystem.TypeFat16:
		return FAT16, true
	case filesystem.TypeFat12:
		return FAT12, true
	default:
		return "", false
	}
}

// trimLabel drops the trailing space/NUL padding FAT stores volume labels with.
func trimLabel(label string) string {
	return strings.TrimRight(label, " \x00")
}

// readLeadingRegion returns the first blankProbeBytes of devicePath, or all of
// it if shorter.
func readLeadingRegion(devicePath string) ([]byte, error) {
	f, err := os.Open(devicePath)
	if err != nil {
		return nil, fmt.Errorf("opening %s to check what it holds failed: %w", devicePath, err)
	}
	defer func() { _ = f.Close() }()

	buf := make([]byte, blankProbeBytes)
	n, err := io.ReadFull(f, buf)
	switch err {
	case nil, io.EOF, io.ErrUnexpectedEOF:
	default:
		return nil, fmt.Errorf("reading the start of %s to check what it holds failed: %w", devicePath, err)
	}
	return buf[:n], nil
}

func isAllZero(data []byte) bool {
	for _, b := range data {
		if b != 0 {
			return false
		}
	}
	return true
}

// Format writes a whole-device filesystem of the requested kind, labelled
// volumeLabel, to the block device (or image file) at devicePath, discarding
// any existing contents.
func Format(devicePath, volumeLabel string, fs FS) error {
	switch fs {
	case FAT32:
		return FormatFAT32(devicePath, volumeLabel)
	case ExFAT:
		return FormatExFAT(devicePath, volumeLabel)
	case EXT4:
		return FormatEXT4(devicePath, volumeLabel)
	default:
		return fmt.Errorf("cannot format %s: %q is not a filesystem GoSD can create", devicePath, string(fs))
	}
}

// FormatFAT32 formats the block device (or image file) at devicePath as a
// single whole-device FAT32 filesystem labelled volumeLabel, discarding any
// existing contents.
//
// It opens the device read-write without O_EXCL (which would fail when the
// kernel already holds a block device) and formats the whole device with no
// partition table, so no partition-table reread — the one step that needs
// privileges on real hardware — is performed. The device's size is detected
// automatically; the caller need not supply it.
func FormatFAT32(devicePath, volumeLabel string) (err error) {
	d, err := openDisk(devicePath, false)
	if err != nil {
		return fmt.Errorf("opening %s for formatting failed: %w", devicePath, err)
	}
	defer func() {
		if cerr := d.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("closing %s after formatting failed: %w", devicePath, cerr)
		}
	}()

	// Before anything is written, so an oversized device is left untouched.
	if err := checkFAT32Size(devicePath, d.Size); err != nil {
		return err
	}

	// go-diskfs sizes the FAT too small for the clusters it then advertises at
	// ~0.8% of volume sizes, so the filesystem spans the largest prefix of the
	// device it lays out correctly — at most two clusters short of all of it.
	// See fat32selfconsistent.go.
	d.Size = LargestSelfConsistentFAT32Bytes(d.Size)

	if _, err := d.CreateFilesystem(disk.FilesystemSpec{
		Partition:   0, // 0 = whole device, no partition table
		FSType:      filesystem.TypeFat32,
		VolumeLabel: volumeLabel,
	}); err != nil {
		return fmt.Errorf("writing a FAT32 filesystem to %s failed: %w", devicePath, err)
	}
	return nil
}

// CreateEmptyFile creates an empty file called name in the root directory of
// the filesystem on devicePath, without mounting it: go-diskfs writes the
// directory entry straight to the device. name is a plain filename, not a
// path, and must NOT begin with a dot — go-diskfs derives an empty 8.3 short
// name for such a name and then skips the entry when listing the directory,
// so the file it creates is one RootFileExists can never find.
//
// The write lands in the page cache like any other; a caller that needs it on
// the medium must flush the device afterwards.
func CreateEmptyFile(devicePath, name string) (err error) {
	d, err := openDisk(devicePath, false)
	if err != nil {
		return fmt.Errorf("opening %s to create %s failed: %w", devicePath, name, err)
	}
	defer func() {
		if cerr := d.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("closing %s after creating %s failed: %w", devicePath, name, cerr)
		}
	}()

	fs, err := d.GetFilesystem(0)
	if err != nil {
		return fmt.Errorf("reading the filesystem on %s failed: %w", devicePath, err)
	}
	f, err := fs.OpenFile("/"+name, os.O_CREATE|os.O_RDWR)
	if err != nil {
		return fmt.Errorf("creating %s on %s failed: %w", name, devicePath, err)
	}
	if cerr := f.Close(); cerr != nil {
		return fmt.Errorf("closing %s on %s failed: %w", name, devicePath, cerr)
	}
	return nil
}

// RootFileExists reports whether the root directory of the filesystem on
// devicePath holds a file called name, matched case-insensitively as FAT
// itself matches names. An error means the directory could not be read at
// all, which is not the same answer as "no". This also covers a directory
// the Linux kernel has written into (the on-device case: gosd-init/apps
// write to devicePath, then a host tool reads it back through here) —
// go-diskfs correctly skips the FAT delete-marker (0xE5) entries a kernel
// rename leaves behind, including on LFN continuation slots; see
// TestRootFileExistsSurvivesKernelDeletedEntries (gosd-zzdz).
func RootFileExists(devicePath, name string) (found bool, err error) {
	d, err := openDisk(devicePath, true)
	if err != nil {
		return false, fmt.Errorf("opening %s to look for %s failed: %w", devicePath, name, err)
	}
	defer func() { _ = d.Close() }()

	fs, err := d.GetFilesystem(0)
	if err != nil {
		return false, fmt.Errorf("reading the filesystem on %s failed: %w", devicePath, err)
	}
	// "." rather than "/": go-diskfs validates directory paths as io/fs
	// paths, which are unrooted, and rejects a leading slash.
	entries, err := fs.ReadDir(".")
	if err != nil {
		return false, fmt.Errorf("reading the root directory of %s failed: %w", devicePath, err)
	}
	for _, e := range entries {
		if strings.EqualFold(e.Name(), name) {
			return true, nil
		}
	}
	return false, nil
}

// openDisk opens the block device (or image file) at devicePath as a
// go-diskfs disk, finding its size by seeking to its end rather than via
// diskfs.Open's ioctl(BLKGETSIZE64). That ioctl reads the kernel's u64 answer
// into a Go int, which on 32-bit ARM (pi-zero-w) is 4 bytes: adjacent stack
// memory is corrupted and any device of 4GiB or more reports a truncated size
// (bean gosd-fjio). lseek's offset is 64-bit on every Linux architecture Go
// supports, works on block devices and regular files alike, and so keeps the
// image-file test path identical to the real-device one.
func openDisk(devicePath string, readOnly bool) (*disk.Disk, error) {
	flag := os.O_RDWR
	if readOnly {
		flag = os.O_RDONLY
	}
	f, err := os.OpenFile(devicePath, flag, 0)
	if err != nil {
		return nil, err
	}
	size, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("finding the size of %s failed: %w", devicePath, err)
	}
	return &disk.Disk{
		Backend:           file.New(f, readOnly),
		Size:              size,
		LogicalBlocksize:  sectorSizeBytes,
		PhysicalBlocksize: sectorSizeBytes,
		DefaultBlocks:     true,
	}, nil
}
