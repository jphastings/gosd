//go:build linux

package dataexpand

import (
	"fmt"
	"io"
	"os"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"

	"github.com/jphastings/gosd/internal/blockmount"
	"github.com/jphastings/gosd/internal/diskfmt"
)

// NewDeps returns Deps wired to the real block-device syscalls, diskfmt and
// blockmount, logging through log.
//
// ext4Mountpoint is where EstablishEXT4 and EXT4Established mount the data
// partition briefly. FAT32 needs no equivalent: go-diskfs reads and writes
// its raw-device marker with nothing mounted at all, but growing an ext4
// filesystem (EXT4_IOC_RESIZE_FS) and writing a file into it both require a
// live kernel mount, so this package needs a mountpoint of its own — one
// that isn't /data itself, since establishment has to complete before
// /data's own mount is ever attempted. /run is already a tmpfs by the time
// Run does anything (mountEarly, boot sequence step 1), so a path under it
// needs no cleanup: it evaporates on reboot. Unused for FAT32 images.
func NewDeps(log func(format string, args ...any), ext4Mountpoint string) Deps {
	return Deps{
		ReadMBR:            readMBR,
		WriteMBR:           writeMBR,
		DeviceSizeBytes:    deviceSizeBytes,
		AddKernelPartition: addKernelPartition,
		Inspect:            diskfmt.Inspect,
		FormatFAT32:        diskfmt.FormatFAT32,
		CreateMarker: func(partitionDevice string) error {
			return diskfmt.CreateEmptyFile(partitionDevice, EstablishedMarker)
		},
		MarkerExists: func(partitionDevice string) (bool, error) {
			return diskfmt.RootFileExists(partitionDevice, EstablishedMarker)
		},
		FormatEXT4: func(partitionDevice, label string) error {
			// The data golden, always: this partition is grown to the
			// card's real size right afterwards (EstablishEXT4 below).
			return diskfmt.FormatEXT4(diskfmt.EXT4GoldenData, partitionDevice, label)
		},
		EstablishEXT4: func(partitionDevice string) error {
			return establishEXT4(partitionDevice, ext4Mountpoint)
		},
		EXT4Established: func(partitionDevice string) (bool, error) {
			return ext4Established(partitionDevice, ext4Mountpoint)
		},
		FilesystemSupported: blockmount.Mountable,
		SyncDevice:          syncDevice,
		PathExists: func(path string) bool {
			_, err := os.Stat(path)
			return err == nil
		},
		Sleep: time.Sleep,
		Now:   time.Now,
		Log:   log,
	}
}

// establishEXT4 mounts partitionDevice's ext4 filesystem at mountpoint,
// grows it to fill the partition, writes and fsyncs the establishment
// marker, then unmounts — on every return path, including an error one, so
// a failed grow or marker write never leaves the partition mounted
// underneath whatever runs next this boot.
func establishEXT4(partitionDevice, mountpoint string) (err error) {
	if err := blockmount.Mount(partitionDevice, mountpoint, diskfmt.EXT4); err != nil {
		return fmt.Errorf("mounting %s at %s to establish its ext4 filesystem: %w", partitionDevice, mountpoint, err)
	}
	defer func() {
		if uerr := blockmount.Unmount(mountpoint); uerr != nil && err == nil {
			err = fmt.Errorf("unmounting %s after establishing its ext4 filesystem: %w", mountpoint, uerr)
		}
	}()
	if err := blockmount.GrowEXT4(partitionDevice, mountpoint); err != nil {
		return fmt.Errorf("growing %s's ext4 filesystem to its partition size: %w", partitionDevice, err)
	}
	if err := blockmount.EstablishMarker(mountpoint, diskfmt.EXT4); err != nil {
		return fmt.Errorf("recording the completed establishment of %s's ext4 filesystem: %w", partitionDevice, err)
	}
	return nil
}

// ext4Established mounts partitionDevice's ext4 filesystem at mountpoint
// just long enough to check for blockmount.EstablishedMarker, then
// unmounts. A mount failure is reported as an error rather than folded into
// a false "not established" result — see Deps.EXT4Established's doc for why
// that distinction matters to its two callers.
func ext4Established(partitionDevice, mountpoint string) (bool, error) {
	if err := blockmount.Mount(partitionDevice, mountpoint, diskfmt.EXT4); err != nil {
		return false, fmt.Errorf("mounting %s at %s to check whether its ext4 filesystem was established: %w", partitionDevice, mountpoint, err)
	}
	defer func() { _ = blockmount.Unmount(mountpoint) }()
	return blockmount.MarkerEstablished(mountpoint)
}

func readMBR(device string) ([]byte, error) {
	f, err := os.Open(device)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	buf := make([]byte, mbrSize)
	if _, err := io.ReadFull(f, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

func writeMBR(device string, sector []byte) (err error) {
	f, err := os.OpenFile(device, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	if _, err := f.WriteAt(sector, 0); err != nil {
		return err
	}
	return f.Sync()
}

// syncDevice fsyncs the device node, flushing its page-cache-buffered
// writes (a freshly written format) to the medium. The partition node's
// pages are its own — a sync of the whole-disk node would not cover them.
func syncDevice(device string) (err error) {
	f, err := os.OpenFile(device, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	return f.Sync()
}

// deviceSizeBytes seeks to the device's end rather than asking
// ioctl(BLKGETSIZE64), whose result is mis-read on 32-bit ARM — the same
// rule internal/diskfmt follows (bean gosd-fjio).
func deviceSizeBytes(device string) (int64, error) {
	f, err := os.Open(device)
	if err != nil {
		return 0, err
	}
	defer func() { _ = f.Close() }()
	return f.Seek(0, io.SeekEnd)
}

// blkpgAddPartition is BLKPG_ADD_PARTITION from linux/blkpg.h.
const blkpgAddPartition = 1

// blkpgPartition mirrors struct blkpg_partition (linux/blkpg.h). The field
// offsets coincide with C's on every Go/Linux architecture; the trailing pad
// covers 32-bit ARM, where C's long long alignment rounds the struct to 152
// bytes but Go's 4-byte int64 alignment would stop at 148 — the kernel
// copies C's sizeof from userspace, so the Go value must span it.
type blkpgPartition struct {
	start   int64
	length  int64
	pno     int32
	devname [64]byte
	volname [64]byte
	_       [4]byte
}

// blkpgIoctlArg mirrors struct blkpg_ioctl_arg (linux/blkpg.h).
type blkpgIoctlArg struct {
	op      int32
	flags   int32
	datalen int32
	data    unsafe.Pointer
}

// addKernelPartition registers the new partition with the running kernel via
// BLKPG_ADD_PARTITION — the single-partition alternative to a BLKRRPART full
// reread, which fails with EBUSY while partition 1 is mounted as /boot.
// devtmpfs then creates the partition's device node.
func addKernelPartition(device string, partNo int, startBytes, sizeBytes int64) error {
	f, err := os.OpenFile(device, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	part := blkpgPartition{start: startBytes, length: sizeBytes, pno: int32(partNo)}
	arg := blkpgIoctlArg{
		op:      blkpgAddPartition,
		datalen: int32(unsafe.Sizeof(part)),
		data:    unsafe.Pointer(&part),
	}
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, f.Fd(), unix.BLKPG, uintptr(unsafe.Pointer(&arg))); errno != 0 {
		return fmt.Errorf("telling the kernel about partition %d on %s failed: %w", partNo, device, errno)
	}
	return nil
}
