//go:build linux

package blockmount

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"

	"github.com/jphastings/gosd/internal/diskfmt"
)

const (
	sysBlockDir      = "/sys/block"
	procMounts       = "/proc/mounts"
	procFilesystems  = "/proc/filesystems"
	vfatMountOptions = "flush" // push writes to a journal-less FAT promptly
)

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

// MountedAt reports whether something is mounted at mountpoint and, if so, the
// device node behind it.
func MountedAt(mountpoint string) (string, bool, error) {
	entries, err := parseMounts()
	if err != nil {
		return "", false, err
	}
	for _, e := range entries {
		if e.target == mountpoint {
			return e.source, true, nil
		}
	}
	return "", false, nil
}

// MountedSources returns the set of device nodes currently mounted, e.g.
// "/dev/mmcblk1p1".
func MountedSources() (map[string]bool, error) {
	entries, err := parseMounts()
	if err != nil {
		return nil, err
	}
	sources := make(map[string]bool, len(entries))
	for _, e := range entries {
		sources[e.source] = true
	}
	return sources, nil
}

// Unmount releases the filesystem mounted at mountpoint. Unmounting an
// already-unmounted mountpoint reports EINVAL, which callers switching modes
// can treat as already-released.
func Unmount(mountpoint string) error {
	if err := unix.Unmount(mountpoint, 0); err != nil {
		return fmt.Errorf("unmounting %s failed: %w", mountpoint, err)
	}
	return nil
}

// Mount mounts device read-write at mountpoint as a filesystem of kind fs,
// creating the mountpoint if it does not exist.
func Mount(device, mountpoint string, fs diskfmt.FS) error {
	fsType := fs.MountType()
	if fsType == "" {
		return fmt.Errorf("cannot mount %s: %q is not a filesystem GoSD knows how to mount", device, string(fs))
	}
	if err := os.MkdirAll(mountpoint, 0o755); err != nil {
		return fmt.Errorf("creating mountpoint %s failed: %w", mountpoint, err)
	}
	if err := unix.Mount(device, mountpoint, fsType, unix.MS_NOSUID|unix.MS_NODEV, mountData(fs)); err != nil {
		return fmt.Errorf("mount(%s, %s, %s) failed: %w", device, mountpoint, fsType, err)
	}
	return nil
}

// mountData is the comma-separated option string for a filesystem. The options
// are per-driver and mount(2) rejects one it does not know, so "flush" — which
// only Linux's vfat driver has — must not reach exfat.
func mountData(fs diskfmt.FS) string {
	if fs == diskfmt.FAT32 {
		return vfatMountOptions
	}
	return ""
}

// Mountable reports whether the running kernel can mount a filesystem of kind
// fs. GoSD kernels are built without loadable modules, so every filesystem the
// kernel will ever have is already listed in /proc/filesystems — which makes
// this an exact answer rather than a guess, and lets a format be refused before
// it destroys anything.
func Mountable(fs diskfmt.FS) (bool, error) {
	fsType := fs.MountType()
	if fsType == "" {
		return false, nil
	}
	raw, err := os.ReadFile(procFilesystems)
	if err != nil {
		return false, fmt.Errorf("reading %s to check for %s support failed: %w", procFilesystems, fs, err)
	}
	// Each line is an optional "nodev" marker then the filesystem name.
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[len(fields)-1] == fsType {
			return true, nil
		}
	}
	return false, nil
}

// ReadBlockDevices enumerates every block device under /sys/block with the
// attributes candidate selection weighs. Deciding which of them may be
// formatted is the caller's Rank function, not this scan's.
func ReadBlockDevices() ([]Device, error) {
	entries, err := os.ReadDir(sysBlockDir)
	if err != nil {
		return nil, fmt.Errorf("reading %s failed: %w", sysBlockDir, err)
	}
	devices := make([]Device, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		devices = append(devices, Device{
			Name:        name,
			Kind:        readAttr(name, "device/type"),
			Partitions:  readPartitions(name),
			SizeSectors: readUint(name, "size"),
			ReadOnly:    readAttr(name, "ro") == "1",
		})
	}
	return devices, nil
}

func readAttr(name, attr string) string {
	raw, err := os.ReadFile(filepath.Join(sysBlockDir, name, attr))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

func readUint(name, attr string) uint64 {
	v, err := strconv.ParseUint(readAttr(name, attr), 10, 64)
	if err != nil {
		return 0
	}
	return v
}

// readPartitions lists a block device's partition node names. Partition naming
// differs by device class ("sda1" but "nvme0n1p1"), so they are identified
// structurally instead: the kernel gives every partition directory under
// /sys/block/<name> a "partition" attribute, and gives it to nothing else.
func readPartitions(name string) []string {
	entries, err := os.ReadDir(filepath.Join(sysBlockDir, name))
	if err != nil {
		return nil
	}
	var parts []string
	for _, entry := range entries {
		if _, err := os.Stat(filepath.Join(sysBlockDir, name, entry.Name(), "partition")); err == nil {
			parts = append(parts, entry.Name())
		}
	}
	return parts
}
