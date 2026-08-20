package blockmount

import (
	"regexp"
	"sort"
)

// Device is one entry under /sys/block, as candidate selection sees it.
type Device struct {
	// Name is the kernel device name, e.g. "mmcblk0", "nvme0n1", "sda".
	Name string
	// Kind is /sys/block/<name>/device/type — "MMC" for a soldered/plug-in
	// eMMC, "SD" for a card, "" where the attribute does not exist (which is
	// the case for NVMe namespaces and SCSI/USB disks).
	Kind string
	// Partitions are the device's partition node names, e.g. "nvme0n1p1" or
	// "sda1".
	Partitions []string
	// SizeSectors is /sys/block/<name>/size, the device's size in 512-byte
	// sectors. Zero means no medium — an empty card-reader slot still
	// enumerates as a block device.
	SizeSectors uint64
	// ReadOnly mirrors /sys/block/<name>/ro: a write-protected card cannot be
	// a format target.
	ReadOnly bool
}

// Rank decides both suitability and preference for a candidate device: ok is
// false for a device that must never be a format target, and otherwise rank
// orders the survivors, lower first.
type Rank func(Device) (rank int, ok bool)

// Candidates returns the device nodes that could be formatted, best first.
// Anything mounted from a device — the whole device or any of its partitions —
// makes it in use, which is what keeps the media the board booted from off the
// list. Usable is checked here, ahead of rank, so a device with no medium or
// that is write-protected can never become a candidate regardless of what any
// package's Rank says — see Usable's doc for why that check lives here rather
// than in each package's Rank. Ties on rank are broken by device name so the
// order never depends on kernel enumeration order.
func Candidates(devices []Device, mountedSources map[string]bool, rank Rank) []string {
	type candidate struct {
		name string
		rank int
	}
	var found []candidate
	for _, dev := range devices {
		if InUse(dev, mountedSources) {
			continue
		}
		if !Usable(dev) {
			continue
		}
		if r, ok := rank(dev); ok {
			found = append(found, candidate{name: dev.Name, rank: r})
		}
	}
	sort.Slice(found, func(i, j int) bool {
		if found[i].rank != found[j].rank {
			return found[i].rank < found[j].rank
		}
		return found[i].name < found[j].name
	})

	paths := make([]string, len(found))
	for i, c := range found {
		paths[i] = "/dev/" + c.name
	}
	return paths
}

// Choose picks the best candidate, or returns none when there isn't one.
func Choose(devices []Device, mountedSources map[string]bool, rank Rank, none error) (string, error) {
	paths := Candidates(devices, mountedSources, rank)
	if len(paths) == 0 {
		return "", none
	}
	return paths[0], nil
}

// InUse reports whether the whole device or any of its partitions is currently
// mounted.
func InUse(dev Device, mountedSources map[string]bool) bool {
	if mountedSources["/dev/"+dev.Name] {
		return true
	}
	for _, part := range dev.Partitions {
		if mountedSources["/dev/"+part] {
			return true
		}
	}
	return false
}

// Usable reports whether dev could ever be a legitimate format target,
// independent of any package's class preference. Three things disqualify a
// device no matter what a caller's Rank would otherwise say: no medium
// (SizeSectors == 0 — an empty card-reader slot still enumerates as a block
// device), write protection (ReadOnly), and being an eMMC hardware partition
// (IsMMCHardwarePartition — boot code, replay-protected storage or
// vendor-managed content, never general storage).
//
// All three live here, in Candidates, rather than in each package's Rank, so
// the two public packages (emmc and disk) cannot silently diverge on them
// again — and they had, twice, in the same direction: disk.rank enforced the
// medium and write-protection checks explicitly while emmc's rank did not
// (gosd-ix38), and disk.rank excluded hardware partitions by an explicit
// pattern (gosd-f226) while emmc's stayed clear of them only by accident,
// because those gendisks happen to report no device/type in sysfs so
// `Kind == "MMC"` misses them (gosd-b6jl). That quirk is not a documented
// kernel contract, was never verified on the Rockchip boards this project
// ships, and would fail silently — formatting an eMMC's boot0 leaves a board
// that no longer boots and cannot be recovered from the SD card. A package's
// Rank now only ever needs to express its own class preference.
func Usable(dev Device) bool {
	return dev.SizeSectors != 0 && !dev.ReadOnly && !IsMMCHardwarePartition(dev.Name)
}

// mmcHardwarePartitionRE matches the block-device names the kernel's MMC
// block driver registers for an eMMC's hardware partitions: boot0/boot1 (boot
// code), rpmb (replay-protected storage) and gp0-gp3 (vendor general-purpose
// areas — on a Rockchip board these typically hold DRM keys, calibration data
// or other secure storage the vendor put there, per gosd-f226). Each is its
// own /sys/block gendisk alongside the user-data area (e.g. mmcblk0gp0 next to
// mmcblk0), so it must be excluded structurally rather than by growing a
// suffix list: a suffix check risks a false positive against a plain
// partition name that happens to end the same way, and would need a new entry
// every time the kernel's naming grows. The digit groups use \d+ rather than
// a literal 0-3/0-1 so an unexpected shape (a double-digit device number, an
// index the kernel doesn't use today) is still caught defensively; the
// anchors keep a name that merely contains "boot"/"rpmb"/"gp" from matching
// by accident. Partitions of the user area (mmcblk0p1) are never mistaken for
// a hardware partition — "p1" is not one of "boot\d+", "rpmb" or "gp\d+".
var mmcHardwarePartitionRE = regexp.MustCompile(`^mmcblk\d+(boot\d+|rpmb|gp\d+)$`)

// IsMMCHardwarePartition spots an eMMC's boot, replay-protected and
// general-purpose hardware partitions, which the kernel exposes as their own
// block devices alongside the user area. Usable rejects every one of them,
// for both public packages; the packages' own Rank functions name it too, so
// that a Rank read on its own is honest about what it does and does not
// accept.
func IsMMCHardwarePartition(name string) bool {
	return mmcHardwarePartitionRE.MatchString(name)
}
