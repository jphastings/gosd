package blockmount

import "sort"

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
// list. Ties on rank are broken by device name so the order never depends on
// kernel enumeration order.
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
