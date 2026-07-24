package boot

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
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
// devtmpfs on /dev, proc on /proc, sysfs on /sys, configfs on
// /sys/kernel/config, tmpfs on /run. configfs is mounted unconditionally
// (not gated behind an app's USB gadget mode use) since it costs nothing
// unused and every supported board's kernel already builds it in as a
// dependency of CONFIG_USB_CONFIGFS — see the gadget package for what gets
// written under it.
var earlyMounts = []mountSpec{
	{source: "devtmpfs", target: "/dev", fstype: "devtmpfs", flags: msNoSuid, data: "mode=0755"},
	{source: "proc", target: "/proc", fstype: "proc", flags: msNoSuid | msNoDev | msNoExec},
	{source: "sysfs", target: "/sys", fstype: "sysfs", flags: msNoSuid | msNoDev | msNoExec},
	{source: "configfs", target: "/sys/kernel/config", fstype: "configfs", flags: msNoSuid | msNoDev | msNoExec},
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

// bootSentinelFile is the file MountBootPartition requires at the root of a
// freshly-mounted candidate before accepting it as GOSD-BOOT, rather than
// accepting the first candidate the kernel is willing to mount as FAT (see
// gosd-pcwl). internal/pipeline.Assemble writes gosd.toml onto every
// board's boot partition unconditionally — unlike config.txt (Pi-only) or
// extlinux.conf (Rockchip-only) — which is what makes it safe to key off
// across the whole board matrix.
const bootSentinelFile = "gosd.toml"

// MountBootPartition mounts the GOSD-BOOT FAT partition read-only at
// target, trying each candidate device in turn, and returns the device it
// mounted from. The MMC controller may still be probing when gosd-init
// reaches this step (no udev is available to wait on), so failures are
// retried for up to timeout before giving up.
//
// A candidate that mounts as valid FAT is not accepted on that basis alone:
// with an eMMC fitted, its first partition can sort before the SD card's in
// device-name order (mmcblk0 vs mmcblk1) and, if it happens to already hold
// a valid FAT filesystem (a previously-flashed GoSD image, or a vendor
// image), would otherwise be silently mounted as /boot instead of the SD
// card the user just flashed. So each successful FAT mount is additionally
// checked for bootSentinelFile at its root; a candidate missing it is
// unmounted and probing moves on to the next one, within the same timeout
// budget as an outright mount failure.
func MountBootPartition(m Mounter, target string, devices []string, timeout time.Duration, pathExists func(path string) bool, sleep func(time.Duration), now func() time.Time) (string, error) {
	deadline := now().Add(timeout)
	var lastErr error
	for {
		for _, dev := range devices {
			if err := m.Mount(dev, target, "vfat", msRdOnly, ""); err != nil {
				lastErr = err
				continue
			}
			if pathExists(path.Join(target, bootSentinelFile)) {
				return dev, nil
			}
			lastErr = fmt.Errorf("%s mounted as a valid FAT filesystem but has no %s at its root: not the GOSD-BOOT partition", dev, bootSentinelFile)
			if err := m.Unmount(target); err != nil {
				return "", fmt.Errorf("unmounting %s after it failed the GOSD-BOOT sentinel check: %w", dev, err)
			}
		}
		if !now().Before(deadline) {
			return "", fmt.Errorf("mounting boot partition at %s failed after retrying for %s (tried %s): %w",
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
// data to storage promptly after writes — FAT has no journal, so the less
// time dirty data sits in RAM on a device with no clean-shutdown story, the
// better.
//
// A round in which every candidate fails with "no such file or directory"
// means the device nodes simply don't exist. This step runs only after the
// boot partition (partition 1 of the same underlying block device - an SD
// card, eMMC, or qemu-virt's virtio-blk disk) has mounted, so the kernel has
// already scanned that device's partition table: a missing p2 node is a
// definitive "this image has no data partition", reported immediately as
// ErrDataPartitionMissing rather than burning the whole timeout on it. This
// holds candidate-by-candidate even as the device list grows (mmcblk0,
// mmcblk1, vda, ...): whichever one the boot partition actually mounted
// from is the one already known-scanned, and only real hardware/VM ever
// exposes one of them at a time, so "every candidate" and "the one that
// matters" are the same check in practice.
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

// MountDataReadOnlyFallback mounts an empty read-only tmpfs over target, used
// when no writable GOSD-DATA partition is available (absent or unmountable).
// It exists so that a write to /data fails loudly instead of silently: /app
// runs as root, whose CAP_DAC_OVERRIDE bypasses directory permission bits, so
// a mode-restricted mountpoint wouldn't stop it — but a read-only superblock
// returns EROFS at the VFS layer, which no capability overrides. Without this,
// /data would be a plain writable directory on the RAM-backed rootfs and any
// write would appear to succeed yet vanish on the next reboot.
func MountDataReadOnlyFallback(m Mounter, target string) error {
	return m.Mount("tmpfs", target, "tmpfs", msRdOnly|msNoSuid|msNoDev, "")
}
