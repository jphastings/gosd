package boot

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"time"
)

// Linux mount(2) flags, mirrored here (rather than imported from
// golang.org/x/sys/unix) because their MS_* constants only exist in the
// linux build of that package; this file has no build tag so it can be
// unit-tested on any host. The bit values themselves are part of the stable
// Linux kernel ABI (linux/fs.h) and don't vary by architecture.
const (
	msRdOnly = 1 << 0 // MS_RDONLY
	msNoSuid = 1 << 1 // MS_NOSUID
	msNoDev  = 1 << 2 // MS_NODEV
	msNoExec = 1 << 3 // MS_NOEXEC
)

// mountSpec describes one of the early filesystem mounts gosd-init performs
// as step 1 of the boot sequence.
type mountSpec struct {
	source, target, fstype string
	flags                  uintptr
	data                   string
}

// earlyMounts are the filesystems gosd-init mounts before anything else:
// devtmpfs on /dev, proc on /proc, sysfs on /sys, tmpfs on /run.
var earlyMounts = []mountSpec{
	{source: "devtmpfs", target: "/dev", fstype: "devtmpfs", flags: msNoSuid, data: "mode=0755"},
	{source: "proc", target: "/proc", fstype: "proc", flags: msNoSuid | msNoDev | msNoExec},
	{source: "sysfs", target: "/sys", fstype: "sysfs", flags: msNoSuid | msNoDev | msNoExec},
	{source: "tmpfs", target: "/run", fstype: "tmpfs", flags: msNoSuid | msNoDev, data: "mode=0755"},
}

// mountEarly performs every early mount in order, stopping at (and
// reporting) the first failure.
func mountEarly(m Mounter) error {
	for _, spec := range earlyMounts {
		if err := m.Mount(spec.source, spec.target, spec.fstype, spec.flags, spec.data); err != nil {
			return fmt.Errorf("mounting %s at %s: %w", spec.fstype, spec.target, err)
		}
	}
	return nil
}

// MountBootPartition mounts the GOSD-BOOT FAT partition read-only at
// target, trying each candidate device in turn. The MMC controller may
// still be probing when gosd-init reaches this step (no udev is available
// to wait on), so failures are retried for up to timeout before giving up.
func MountBootPartition(m Mounter, target string, devices []string, timeout time.Duration, sleep func(time.Duration), now func() time.Time) error {
	deadline := now().Add(timeout)
	var lastErr error
	for {
		for _, dev := range devices {
			if err := m.Mount(dev, target, "vfat", msRdOnly, ""); err == nil {
				return nil
			} else {
				lastErr = err
			}
		}
		if !now().Before(deadline) {
			return fmt.Errorf("mounting boot partition at %s failed after retrying for %s (tried %s): %w",
				target, timeout, strings.Join(devices, ", "), lastErr)
		}
		sleep(250 * time.Millisecond)
	}
}

// ErrDataPartitionMissing reports that no candidate GOSD-DATA device node
// exists at all: the image was built without a data partition
// (--data-size=0, or an image from before the partition existed). Callers
// treat this as "no persistent storage", never as a boot failure.
var ErrDataPartitionMissing = errors.New("no data partition device exists")

// MountDataPartition mounts the GOSD-DATA FAT partition read-write at
// target, trying each candidate device in turn with the same retry pattern
// as MountBootPartition. The vfat "flush" option makes the driver push file
// data to the card promptly after writes — FAT has no journal, so the less
// time dirty data sits in RAM on a device with no clean-shutdown story, the
// better.
//
// A round in which every candidate fails with "no such file or directory"
// means the device nodes simply don't exist. This step runs only after the
// boot partition (partition 1 of the same card) has mounted, so the kernel
// has already scanned the card's partition table: a missing p2 node is a
// definitive "this image has no data partition", reported immediately as
// ErrDataPartitionMissing rather than burning the whole timeout on it.
func MountDataPartition(m Mounter, target string, devices []string, timeout time.Duration, sleep func(time.Duration), now func() time.Time) error {
	deadline := now().Add(timeout)
	var lastErr error
	for {
		allMissing := true
		for _, dev := range devices {
			err := m.Mount(dev, target, "vfat", msNoSuid|msNoDev, "flush")
			if err == nil {
				return nil
			}
			lastErr = err
			if !errors.Is(err, fs.ErrNotExist) {
				allMissing = false
			}
		}
		if allMissing {
			return fmt.Errorf("%w (tried %s)", ErrDataPartitionMissing, strings.Join(devices, ", "))
		}
		if !now().Before(deadline) {
			return fmt.Errorf("mounting data partition at %s failed after retrying for %s (tried %s): %w",
				target, timeout, strings.Join(devices, ", "), lastErr)
		}
		sleep(250 * time.Millisecond)
	}
}
