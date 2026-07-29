// Package emmc lets a GoSD app use the onboard eMMC storage on boards that
// have it (the Rockchip boards — Radxa Zero 3E, NanoPi Zero2), formatting it on
// first use and mounting it on every subsequent boot.
//
// Unlike the microSD card the board boots from, the eMMC is soldered on and
// ships blank, so it cannot be formatted on another machine. FormatAndMount
// therefore formats it in place — a whole-device FAT filesystem, the only
// format these boards' kernels can mount — and is idempotent across runs: once
// an eMMC carries a FAT filesystem with the app's chosen label, later runs only
// mount it. FAT is not power-loss-robust and has no unix permissions or
// symlinks; write with the temp-file-then-rename pattern as for GOSD_DATA.
//
// Formatting is destructive, so it is gated: FormatAndMount will format a blank
// eMMC freely, but refuses to overwrite anything else (a FAT volume with a
// different label, or non-FAT content such as a partition table) unless the
// caller explicitly opts in, returning an error wrapping ErrRefusedFormat
// otherwise.
//
// For any other mass storage — an NVMe SSD, a USB drive, an SD card in a reader
// — see the sibling disk package, which has the same shape.
package emmc

import (
	"errors"

	"github.com/jphastings/gosd/internal/blockmount"
	"github.com/jphastings/gosd/internal/diskfmt"
)

// ErrNoEMMC reports that the board has no onboard eMMC available to format and
// mount — either it has none at all (e.g. a Raspberry Pi board), or the only
// eMMC present is the device the board booted from and so is off-limits.
var ErrNoEMMC = errors.New("no onboard eMMC found")

// ErrRefusedFormat reports that the eMMC already holds other content — a FAT
// volume with a different label, or non-FAT content — and destructive was
// false, so FormatAndMount left it untouched instead of wiping it. Callers
// that want to offer the user a way to consent (e.g. an app-env var read from
// gosd.toml's [env] table) can match this with errors.Is and retry with
// destructive=true once they have it.
var ErrRefusedFormat = blockmount.ErrRefusedFormat

// FormatAndMount ensures the board's onboard eMMC carries a FAT filesystem
// labelled label and mounts it read-write at mountpoint, then reports the
// outcome on the returned channel.
//
// It returns immediately; the work runs in the background so the caller can
// continue starting up. The channel receives exactly one Result and is then
// closed. A typical caller blocks on it only when it first needs the storage:
//
//	res := <-emmc.FormatAndMount("APPDATA", "/storage", false)
//	if res.Err != nil {
//		log.Printf("no persistent storage: %v", res.Err)
//	}
//	// res.MountPoint is ready to use; res.BlockDevice is the node behind it.
//
// The eMMC is discovered automatically. An eMMC already FAT-formatted with
// label is only mounted, never reformatted — this is how re-runs of the same
// app avoid wiping their own data. A blank eMMC (no filesystem and an all-zero
// leading region) is always formatted.
//
// destructive governs only an eMMC that already holds *other* data — a FAT
// volume with a different label, or non-FAT content: false makes FormatAndMount
// fail without touching it, wrapping ErrRefusedFormat; true wipes and
// reformats it. label is limited to 11 ASCII characters (the FAT maximum).
func FormatAndMount(label, mountpoint string, destructive bool) <-chan Result {
	out := make(chan Result, 1)
	go func() {
		device, err := blockmount.Run(storage(newPlatformDeps()), diskfmt.FAT32, label, mountpoint, destructive)
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
	// MountPoint is where the eMMC's filesystem is mounted read-write — the
	// mountpoint passed to FormatAndMount.
	MountPoint string
	// BlockDevice is the device node backing MountPoint, e.g. "/dev/mmcblk0".
	// The eMMC carries a whole-device FAT filesystem (no partition table), so
	// this whole-device node can be handed straight to gadget.MassStorage to
	// share over USB — but Unmount MountPoint first: expose the device or mount
	// it, never both at once.
	BlockDevice string
	// Err is non-nil if the eMMC could not be formatted and mounted, including
	// ErrNoEMMC when the board has none and ErrRefusedFormat when it already
	// holds other content and destructive was false.
	Err error
}

// storage describes the onboard eMMC to the shared orchestration.
func storage(d blockmount.Deps) blockmount.Storage {
	return blockmount.Storage{Pkg: "emmc", Noun: "eMMC", Deps: d}
}

// chooseEMMC picks the onboard eMMC from the block devices present. It selects
// the eMMC (device/type "MMC", which distinguishes soldered eMMC from the "SD"
// card, independent of mmcblk numbering) that the board is not currently
// running from — a device with any mounted partition is the boot device and is
// never a format target. mountedSources holds the device nodes currently
// mounted (e.g. "/dev/mmcblk1p1"), so booting from the eMMC safely yields
// ErrNoEMMC rather than a wiped system.
func chooseEMMC(devices []blockmount.Device, mountedSources map[string]bool) (string, error) {
	rank := func(dev blockmount.Device) (int, bool) { return 0, dev.Kind == "MMC" }
	return blockmount.Choose(devices, mountedSources, rank, ErrNoEMMC)
}
