//go:build linux

package emmc

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/sys/unix"

	"github.com/jphastings/gosd/internal/emmcfmt"
)

const (
	sysBlockDir  = "/sys/block"
	procMounts   = "/proc/mounts"
	mountFSType  = "vfat"
	mountOptions = "flush" // push writes to a journal-less FAT promptly
)

// newPlatformDeps wires the real eMMC operations. inspect and format come from
// internal/emmcfmt (pure go-diskfs, no syscalls); discovery, the mount-state
// check, and the mount itself are Linux syscalls/sysfs reads.
func newPlatformDeps() deps {
	return deps{
		mountedAt: mountedAt,
		discover:  discoverEMMC,
		inspect:   emmcfmt.Inspect,
		format:    emmcfmt.FormatFAT32,
		mount:     mountVFAT,
	}
}

// mountEntry is one line of /proc/mounts: the device node and where it is
// mounted.
type mountEntry struct {
	source string
	target string
}

func parseMounts() ([]mountEntry, error) {
	raw, err := os.ReadFile(procMounts)
	if err != nil {
		return nil, fmt.Errorf("reading %s failed: %w", procMounts, err)
	}
	var entries []mountEntry
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		entries = append(entries, mountEntry{source: unescapeMount(fields[0]), target: unescapeMount(fields[1])})
	}
	return entries, nil
}

// unescapeMount reverses the octal escaping (\040 for space, etc.) the kernel
// applies to whitespace in /proc/mounts fields.
func unescapeMount(field string) string {
	if !strings.Contains(field, `\`) {
		return field
	}
	replacer := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)
	return replacer.Replace(field)
}

func mountedAt(mountpoint string) (bool, error) {
	entries, err := parseMounts()
	if err != nil {
		return false, err
	}
	for _, e := range entries {
		if e.target == mountpoint {
			return true, nil
		}
	}
	return false, nil
}

func discoverEMMC() (string, error) {
	devices, err := readBlockDevices()
	if err != nil {
		return "", err
	}
	entries, err := parseMounts()
	if err != nil {
		return "", err
	}
	mountedSources := make(map[string]bool, len(entries))
	for _, e := range entries {
		mountedSources[e.source] = true
	}
	return chooseEMMC(devices, mountedSources)
}

// readBlockDevices enumerates the MMC block devices under /sys/block, reading
// each one's type and partitions.
func readBlockDevices() ([]blockDevice, error) {
	entries, err := os.ReadDir(sysBlockDir)
	if err != nil {
		return nil, fmt.Errorf("reading %s failed: %w", sysBlockDir, err)
	}
	var devices []blockDevice
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "mmcblk") {
			continue
		}
		devices = append(devices, blockDevice{
			name:       name,
			kind:       readDeviceType(name),
			partitions: readPartitions(name),
		})
	}
	return devices, nil
}

// readDeviceType returns the contents of /sys/block/<name>/device/type ("MMC"
// for eMMC, "SD" for a card), or "" if the attribute is absent.
func readDeviceType(name string) string {
	raw, err := os.ReadFile(sysBlockDir + "/" + name + "/device/type")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

// readPartitions lists the partition node names of a block device, which the
// kernel exposes as child directories named <name>pN under /sys/block/<name>.
func readPartitions(name string) []string {
	entries, err := os.ReadDir(sysBlockDir + "/" + name)
	if err != nil {
		return nil
	}
	prefix := name + "p"
	var parts []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), prefix) {
			parts = append(parts, entry.Name())
		}
	}
	return parts
}

func mountVFAT(device, mountpoint string) error {
	if err := os.MkdirAll(mountpoint, 0o755); err != nil {
		return fmt.Errorf("creating mountpoint %s failed: %w", mountpoint, err)
	}
	if err := unix.Mount(device, mountpoint, mountFSType, unix.MS_NOSUID|unix.MS_NODEV, mountOptions); err != nil {
		return fmt.Errorf("mount(%s, %s) failed: %w", device, mountpoint, err)
	}
	return nil
}
