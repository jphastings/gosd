//go:build linux

package dataexpand

import (
	"fmt"
	"io"
	"os"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"

	"github.com/jphastings/gosd/internal/diskfmt"
)

// NewDeps returns Deps wired to the real block-device syscalls and diskfmt,
// logging through log.
func NewDeps(log func(format string, args ...any)) Deps {
	return Deps{
		ReadMBR:            readMBR,
		WriteMBR:           writeMBR,
		DeviceSizeBytes:    deviceSizeBytes,
		AddKernelPartition: addKernelPartition,
		Inspect:            diskfmt.Inspect,
		FormatFAT32:        diskfmt.FormatFAT32,
		PathExists: func(path string) bool {
			_, err := os.Stat(path)
			return err == nil
		},
		Sleep: time.Sleep,
		Now:   time.Now,
		Log:   log,
	}
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
