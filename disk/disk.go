// Package disk lets a GoSD app use an attached mass-storage disk — an NVMe SSD
// in an M.2 slot, a USB drive, an SD card in a reader — formatting it on first
// use and mounting it on every subsequent boot.
//
// It is the general-purpose sibling of the emmc package, which addresses one
// specific device (a board's soldered-on eMMC); disk takes whatever suitable
// mass storage it finds that the board did not boot from. The two have the same
// shape, and the same consequences: FormatAndMount writes a whole-device FAT
// filesystem — no partition table — and is idempotent across runs, so once a
// disk carries a FAT filesystem with the app's chosen label, later runs only
// mount it. FAT is not power-loss-robust and has no unix permissions or
// symlinks; write with the temp-file-then-rename pattern as for GOSD_DATA, and
// note that no single file may exceed FAT32's 4 GiB ceiling however large the
// disk is.
//
// Formatting is destructive, so it is gated: FormatAndMount will format a blank
// disk freely, but refuses to overwrite anything else (a FAT volume with a
// different label, or another filesystem such as the exFAT a drive is likely to
// arrive with) unless the caller explicitly opts in, returning an error
// wrapping ErrRefusedFormat otherwise.
package disk

import (
	"errors"
	"fmt"
	"strings"

	"github.com/jphastings/gosd/internal/blockmount"
)

// ErrNoDisk reports that no usable mass-storage disk was found: either nothing
// suitable is attached, or the only disk present is the one the board booted
// from and so is off-limits. Apps that can run without their disk should match
// this with errors.Is and carry on.
var ErrNoDisk = errors.New("no usable disk found")

// ErrRefusedFormat reports that the disk already holds other content — a FAT
// volume with a different label, or another filesystem — and destructive was
// false, so FormatAndMount left it untouched instead of wiping it. Callers
// that want to offer the user a way to consent (e.g. an app-env var read from
// gosd.toml's [env] table) can match this with errors.Is and retry with
// destructive=true once they have it.
var ErrRefusedFormat = blockmount.ErrRefusedFormat

// FormatAndMount ensures an attached disk carries a FAT filesystem labelled
// label and mounts it read-write at mountpoint, then reports the outcome on the
// returned channel.
//
// It returns immediately; the work runs in the background so the caller can
// continue starting up. The channel receives exactly one Result and is then
// closed. A typical caller blocks on it only when it first needs the storage:
//
//	res := <-disk.FormatAndMount("APPDATA", "/storage", false)
//	if res.Err != nil {
//		log.Printf("no bulk storage: %v", res.Err)
//	}
//	// res.MountPoint is ready to use; res.BlockDevice is the node behind it.
//
// The disk is discovered automatically — see Devices for exactly which block
// devices qualify and in what order they are preferred. A disk already
// FAT-formatted with label is only mounted, never reformatted, which is how
// re-runs of the same app avoid wiping their own data. A blank disk (no
// filesystem and an all-zero leading region) is always formatted.
//
// destructive governs only a disk that already holds *other* data: false makes
// FormatAndMount fail without touching it, wrapping ErrRefusedFormat; true
// wipes and reformats it. label is limited to 11 ASCII characters (the FAT
// maximum).
func FormatAndMount(label, mountpoint string, destructive bool) <-chan Result {
	return formatAndMount(discover, label, mountpoint, destructive)
}

// FormatAndMountDevice is FormatAndMount aimed at one named block device, e.g.
// "/dev/nvme0n1", for an app that has more than one disk attached and knows
// which it wants. Everything else is identical, including the refusal to touch
// a device that is currently in use — so naming the board's boot device by hand
// cannot wipe the running system.
func FormatAndMountDevice(device, label, mountpoint string, destructive bool) <-chan Result {
	return formatAndMount(func() (string, error) { return verifyNamedDevice(device) }, label, mountpoint, destructive)
}

func formatAndMount(discover func() (string, error), label, mountpoint string, destructive bool) <-chan Result {
	out := make(chan Result, 1)
	go func() {
		deps := newPlatformDeps()
		deps.Discover = discover
		device, err := blockmount.Run(storage(deps), label, mountpoint, destructive)
		if err != nil {
			out <- Result{Err: err}
		} else {
			out <- Result{MountPoint: mountpoint, BlockDevice: device}
		}
		close(out)
	}()
	return out
}

// Result is the outcome of a FormatAndMount, delivered once on its channel. On
// success Err is nil and MountPoint/BlockDevice name the ready filesystem and
// the device behind it; on failure Err explains why and the other fields are
// empty.
type Result struct {
	// MountPoint is where the disk's filesystem is mounted read-write — the
	// mountpoint passed to FormatAndMount.
	MountPoint string
	// BlockDevice is the device node backing MountPoint, e.g. "/dev/nvme0n1".
	// The disk carries a whole-device FAT filesystem (no partition table), so
	// this whole-device node can be handed straight to gadget.MassStorage to
	// share over USB — but Unmount MountPoint first: expose the device or mount
	// it, never both at once.
	BlockDevice string
	// Err is non-nil if the disk could not be formatted and mounted, including
	// ErrNoDisk when there is none and ErrRefusedFormat when it already holds
	// other content and destructive was false.
	Err error
}

// storage describes an attached disk to the shared orchestration.
func storage(d blockmount.Deps) blockmount.Storage {
	return blockmount.Storage{Pkg: "disk", Noun: "disk", Deps: d}
}

// deviceClasses are the block-device name prefixes that may be formatted, best
// first. Selection is an allowlist because /sys/block is full of nodes that
// would be catastrophic or pointless to format: loop* (files, not media),
// ram*/zram*/zd* (volatile RAM-backed), dm-* (device-mapper nodes — formatting
// one corrupts whatever it maps), md* (RAID members), sr*/scd* (optical),
// nbd* (network block devices) and mtdblock*/ubiblock* (raw-flash translation
// layers). The order prefers the deliberately-fitted, high-capacity device and
// leaves onboard MMC last, since the emmc package addresses that directly.
var deviceClasses = []string{
	"nvme", // NVMe namespaces, e.g. nvme0n1
	"sd",   // SCSI/USB mass storage, e.g. sda — USB sticks and enclosures
	"vd",   // virtio disks, e.g. vda
	"mmcblk",
}

// rank accepts a block device as a format target and orders it against the
// others. Beyond the class allowlist it rejects an eMMC's boot/RPMB hardware
// partitions (which hold boot code, not general storage), a device reporting no
// medium (an empty card-reader slot still enumerates), and a write-protected
// device — better to report ErrNoDisk than to fail deep inside a format.
func rank(dev blockmount.Device) (int, bool) {
	if dev.SizeSectors == 0 || dev.ReadOnly || isMMCHardwarePartition(dev.Name) {
		return 0, false
	}
	for i, prefix := range deviceClasses {
		if hasClassPrefix(dev.Name, prefix) {
			return i, true
		}
	}
	return 0, false
}

// hasClassPrefix reports whether name belongs to a device class, requiring
// something after the prefix so a bare "sd" or "nvme" never matches.
func hasClassPrefix(name, prefix string) bool {
	return len(name) > len(prefix) && strings.HasPrefix(name, prefix)
}

// isMMCHardwarePartition spots an eMMC's boot and replay-protected areas, which
// the kernel exposes as their own block devices alongside the user area.
func isMMCHardwarePartition(name string) bool {
	for _, suffix := range []string{"boot0", "boot1", "rpmb"} {
		if len(name) > len(suffix) && strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}

// choose picks the disk to format from the block devices present: the
// best-ranked candidate the board is not currently running from, since a device
// with anything mounted from it is in use and never a format target.
func choose(devices []blockmount.Device, mountedSources map[string]bool) (string, error) {
	return blockmount.Choose(devices, mountedSources, rank, ErrNoDisk)
}

// candidates lists every device node that could be formatted, in the order
// FormatAndMount would prefer them.
func candidates(devices []blockmount.Device, mountedSources map[string]bool) []string {
	return blockmount.Candidates(devices, mountedSources, rank)
}

// verifyNamed checks a device a caller named explicitly. It skips the class
// allowlist — an explicit choice is an explicit choice — but keeps the in-use
// rule, which is the one that stops an app wiping the media it booted from.
func verifyNamed(device string, devices []blockmount.Device, mountedSources map[string]bool) (string, error) {
	for _, dev := range devices {
		if "/dev/"+dev.Name != device {
			continue
		}
		if blockmount.InUse(dev, mountedSources) {
			return "", fmt.Errorf("the disk at %s is in use — something is mounted from it, so formatting it would destroy a filesystem in use; unmount it first, or choose another of %v", device, candidates(devices, mountedSources))
		}
		return device, nil
	}
	return "", fmt.Errorf("%w: there is no block device %s attached; the usable ones are %v", ErrNoDisk, device, candidates(devices, mountedSources))
}
