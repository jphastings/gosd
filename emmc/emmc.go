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
// caller explicitly opts in.
package emmc

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/jphastings/gosd/internal/emmcfmt"
)

// ErrNoEMMC reports that the board has no onboard eMMC available to format and
// mount — either it has none at all (e.g. a Raspberry Pi board), or the only
// eMMC present is the device the board booted from and so is off-limits.
var ErrNoEMMC = errors.New("no onboard eMMC found")

// maxFATLabelLen is the FAT volume-label limit (11 bytes). FAT also stores
// labels upper-cased; FormatAndMount matches them case-insensitively so the
// label a caller passes round-trips regardless.
const maxFATLabelLen = 11

// FormatAndMount ensures the board's onboard eMMC carries a FAT filesystem
// labelled label and mounts it read-write at mountpoint, then reports the
// outcome on the returned channel.
//
// It returns immediately; the work runs in the background so the caller can
// continue starting up. The channel receives exactly one value and is then
// closed: nil once mountpoint is ready to use, or an error explaining why it is
// not. A typical caller blocks on it only when it first needs the storage:
//
//	if err := <-emmc.FormatAndMount("APPDATA", "/storage", false); err != nil {
//		log.Printf("no persistent storage: %v", err)
//	}
//
// The eMMC is discovered automatically. An eMMC already FAT-formatted with
// label is only mounted, never reformatted — this is how re-runs of the same
// app avoid wiping their own data. A blank eMMC (no filesystem and an all-zero
// leading region) is always formatted.
//
// destructive governs only an eMMC that already holds *other* data — a FAT
// volume with a different label, or non-FAT content: false makes FormatAndMount
// fail without touching it; true wipes and reformats it. label is limited to 11
// ASCII characters (the FAT maximum).
func FormatAndMount(label, mountpoint string, destructive bool) <-chan error {
	out := make(chan error, 1)
	go func() {
		out <- run(newPlatformDeps(), label, mountpoint, destructive)
		close(out)
	}()
	return out
}

// deps are the side-effecting operations run needs, injected so the
// orchestration can be tested without a real eMMC. The real implementations are
// assembled by newPlatformDeps in platform_linux.go.
type deps struct {
	// mountedAt reports whether something is already mounted at mountpoint.
	mountedAt func(mountpoint string) (bool, error)
	// discover returns the device node of the onboard eMMC, or ErrNoEMMC.
	discover func() (string, error)
	// inspect reports what already occupies the device.
	inspect func(device string) (emmcfmt.Contents, error)
	// format writes a whole-device FAT filesystem labelled label to device.
	format func(device, label string) error
	// mount mounts device read-write at mountpoint.
	mount func(device, mountpoint string) error
}

// run is the pure orchestration behind FormatAndMount: decide, from what is
// already present, whether to mount-only, format, or refuse.
func run(d deps, label, mountpoint string, destructive bool) error {
	if err := validateLabel(label); err != nil {
		return err
	}

	// Warm restart (app relaunched without a reboot): the eMMC is still
	// mounted, so there is nothing to do.
	if mounted, err := d.mountedAt(mountpoint); err != nil {
		return err
	} else if mounted {
		return nil
	}

	device, err := d.discover()
	if err != nil {
		return err
	}

	contents, err := d.inspect(device)
	if err != nil {
		return fmt.Errorf("inspecting the eMMC at %s failed: %w", device, err)
	}

	switch {
	case contents.IsFAT && strings.EqualFold(contents.Label, label):
		// Already provisioned by an earlier run — mount only.
	case contents.Blank:
		if err := d.format(device, label); err != nil {
			return fmt.Errorf("formatting the blank eMMC at %s failed: %w", device, err)
		}
	case !destructive:
		return fmt.Errorf("the eMMC at %s already holds %s; refusing to reformat it as %q without permission — pass destructive=true to wipe it", device, describe(contents), label)
	default:
		if err := d.format(device, label); err != nil {
			return fmt.Errorf("reformatting the eMMC at %s failed: %w", device, err)
		}
	}

	if err := d.mount(device, mountpoint); err != nil {
		return fmt.Errorf("mounting the eMMC at %s onto %s failed: %w", device, mountpoint, err)
	}
	return nil
}

// describe renders what is on the eMMC for the "refusing to reformat" error.
func describe(c emmcfmt.Contents) string {
	if c.IsFAT {
		return fmt.Sprintf("a FAT volume labelled %q", c.Label)
	}
	return "non-FAT content"
}

func validateLabel(label string) error {
	if label == "" {
		return errors.New("emmc: the volume label must not be empty")
	}
	if len(label) > maxFATLabelLen {
		return fmt.Errorf("emmc: volume label %q is %d characters; FAT labels are at most %d", label, len(label), maxFATLabelLen)
	}
	for _, r := range label {
		if r > unicode.MaxASCII || !unicode.IsPrint(r) {
			return fmt.Errorf("emmc: volume label %q must be printable ASCII", label)
		}
	}
	return nil
}

// blockDevice is one entry under /sys/block that chooseEMMC weighs.
type blockDevice struct {
	// name is the kernel device name, e.g. "mmcblk0".
	name string
	// kind is /sys/block/<name>/device/type — "MMC" for eMMC, "SD" for a
	// card, "" if the attribute is absent.
	kind string
	// partitions are the device's partition node names, e.g. "mmcblk0p1".
	partitions []string
}

// chooseEMMC picks the onboard eMMC from the block devices present. It selects
// the eMMC (device/type "MMC", which distinguishes soldered eMMC from the "SD"
// card, independent of mmcblk numbering) that the board is not currently
// running from — a device with any mounted partition is the boot device and is
// never a format target. mountedSources holds the device nodes currently
// mounted (e.g. "/dev/mmcblk1p1"), so booting from the eMMC safely yields
// ErrNoEMMC rather than a wiped system.
func chooseEMMC(devices []blockDevice, mountedSources map[string]bool) (string, error) {
	var candidates []string
	for _, dev := range devices {
		if dev.kind != "MMC" || inUse(dev, mountedSources) {
			continue
		}
		candidates = append(candidates, dev.name)
	}
	if len(candidates) == 0 {
		return "", ErrNoEMMC
	}
	sort.Strings(candidates)
	return "/dev/" + candidates[0], nil
}

// inUse reports whether the whole device or any of its partitions is currently
// mounted.
func inUse(dev blockDevice, mountedSources map[string]bool) bool {
	if mountedSources["/dev/"+dev.name] {
		return true
	}
	for _, part := range dev.partitions {
		if mountedSources["/dev/"+part] {
			return true
		}
	}
	return false
}
