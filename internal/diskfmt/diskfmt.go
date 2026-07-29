// Package diskfmt inspects and formats whole block devices on-device, in pure
// Go (CGO_ENABLED=0), with no external mkfs and no root beyond write access to
// the device node. It backs the public emmc and disk packages.
//
// FAT32 comes from github.com/diskfs/go-diskfs — already used to build image
// files in internal/image — which can target a real block device: diskfs.Open
// on a device node auto-detects its size via ioctl(BLKGETSIZE64), and
// CreateFilesystem with Partition 0 writes a whole-device FAT32 with no
// partition table (which also avoids the BLKRRPART reread that needs
// privileges). go-diskfs has no exFAT support at all, so exFAT is read and
// written here directly against the Microsoft exFAT specification — see
// exfat.go and exfatformat.go.
package diskfmt

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/diskfs/go-diskfs"
	"github.com/diskfs/go-diskfs/disk"
	"github.com/diskfs/go-diskfs/filesystem"
)

// FS names a filesystem diskfmt can identify, create and mount.
type FS string

const (
	// FAT32 is the universal default: every host mounts it, and it needs no
	// kernel support beyond the CONFIG_VFAT_FS every GoSD board already has.
	FAT32 FS = "fat32"
	// ExFAT lifts FAT32's 4 GiB per-file ceiling, at the cost of needing
	// CONFIG_EXFAT_FS in the board's kernel.
	ExFAT FS = "exfat"
)

// MountType is the name mount(2) knows this filesystem by, which is not always
// the name people call it: FAT32 is Linux's "vfat". Empty for an FS value
// GoSD does not handle.
func (f FS) MountType() string {
	switch f {
	case FAT32:
		return "vfat"
	case ExFAT:
		return "exfat"
	default:
		return ""
	}
}

// String names the filesystem the way an app author would write it.
func (f FS) String() string {
	switch f {
	case FAT32:
		return "FAT32"
	case ExFAT:
		return "exFAT"
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
	return Contents{FS: FAT32, Label: trimLabel(fs.Label())}, true, nil
}

func isFAT(t filesystem.Type) bool {
	return t == filesystem.TypeFat32 || t == filesystem.TypeFat16 || t == filesystem.TypeFat12
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
	default:
		return fmt.Errorf("cannot format %s: %q is not a filesystem GoSD can create", devicePath, string(fs))
	}
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
