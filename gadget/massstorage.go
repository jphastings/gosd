package gadget

import (
	"fmt"
	"strings"
)

// MassStorage is a USB mass-storage Function (configfs f_mass_storage): it
// exposes one LUN, backed by a block device or disk-image file on the
// board, as a removable-drive-style disk on the host. Requires the board's
// kernel to carry CONFIG_USB_CONFIGFS_MASS_STORAGE=y — see COMPATIBILITY.md's
// USB gadget footnote for per-board status.
//
// While the gadget is applied the host owns the backing store outright,
// caching and writing raw blocks with no coordination — the app must not
// mount or write Path itself at the same time: expose or mount, never both.
// Create enforces this: it refuses a Path that is currently mounted, is a
// partition of a currently-mounted device, or is the parent device of a
// currently-mounted partition, naming the mountpoint so the caller knows
// what to Unmount first. A single LUN covers GoSD's use cases today;
// f_mass_storage itself supports additional lun.N directories, a possible
// future extension.
//
// # A LUN is the whole volume
//
// This is block-level sharing: the host gets every byte of Path, so every
// file on it, whether or not the app meant to publish it and whether or not
// its name starts with a dot. There is no way to expose a subdirectory.
// Only back a LUN with a volume the app owns outright — in particular, not
// the data partition GoSD mounts at /data, which carries gosd-init's copy
// of this device's settings (the WiFi passphrase and any ingress token
// among them, in plain text) so that they survive a re-flash. Sharing that
// hands them to whatever computer the cable reaches, and read-write access
// lets that computer plant files that outlive the owner's next re-flash.
// See the USB gadget section of GoSD's runtime documentation, and
// examples/usbwebsite for how an app can hold that line and still be
// editable over USB.
type MassStorage struct {
	// Path is the block device (e.g. /dev/nvme0n1p1) or disk-image file
	// backing the LUN. Required.
	Path string
	// ReadOnly write-protects the LUN: the host can read but not modify it.
	//
	// Its zero value is false, so an omitted field grants an unauthenticated
	// host write access to the whole backing store — the same as a blank USB
	// stick, which is what a mass-storage gadget is. That is often what you
	// want, but it should never be what you got by accident: write the field
	// out either way. ReadOnly is not a confidentiality control — it stops
	// writes, not reads, and the host can still read everything on Path.
	ReadOnly bool
	// Removable reports the medium as removable (like a USB thumb drive),
	// so the host offers a clean eject.
	Removable bool

	// mountedTargets reports every device node currently mounted, mapped
	// to its mountpoint (source -> target, the shape of
	// blockmount.MountedTargets). nil (the zero value) means "ask the
	// board's real /proc/mounts" — see platform_linux.go/platform_other.go
	// for the default; tests in this package override it directly to
	// exercise the mounted-device rejection below without real storage.
	mountedTargets func() (map[string]string, error)
}

// Name implements Function. "usb0" is this gadget's only mass-storage
// instance, matching ACM's instance-naming convention.
func (MassStorage) Name() string { return "mass_storage.usb0" }

// Create implements Function, writing the LUN's attribute files. The kernel
// creates lun.0 itself as a configfs default group when the function
// directory is made, so only the attributes inside it are written here —
// flags before file, because the kernel refuses to change them once a
// backing file is open.
func (m MassStorage) Create(fsys writableFS, dir string) error {
	if m.Path == "" {
		return fmt.Errorf("MassStorage.Path is empty; set it to the block device or disk-image file the LUN should expose")
	}

	targetsFn := m.mountedTargets
	if targetsFn == nil {
		targetsFn = defaultMountedTargets
	}
	targets, err := targetsFn()
	if err != nil {
		return fmt.Errorf("MassStorage: checking whether %s is already mounted: %w", m.Path, err)
	}
	if mountpoint, blocked := mountedAt(m.Path, targets); blocked {
		return fmt.Errorf("MassStorage.Path %s is already mounted at %s; the board's filesystem cache and the USB host would both write raw blocks to it with no coordination — Unmount(%q) first, then Apply", m.Path, mountpoint, mountpoint)
	}

	lun := dir + "/lun.0"
	attrs := []struct{ name, value string }{
		{"ro", boolAttr(m.ReadOnly)},
		{"removable", boolAttr(m.Removable)},
		{"file", m.Path + "\n"},
	}
	for _, attr := range attrs {
		path := lun + "/" + attr.name
		if err := fsys.WriteFile(path, []byte(attr.value), 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", path, err)
		}
	}
	return nil
}

// boolAttr renders b the way configfs boolean attributes expect it written.
func boolAttr(b bool) string {
	if b {
		return "1\n"
	}
	return "0\n"
}

// mountedAt reports whether path is blocked from being exposed as a LUN
// because it, or a device it shares a physical disk with, appears as a
// mount source in targets (source -> mountpoint, e.g. from
// blockmount.MountedTargets): path itself, a partition of path, or path's
// own parent device. It returns the first matching mountpoint, so Create
// can name it in an actionable error.
func mountedAt(path string, targets map[string]string) (mountpoint string, blocked bool) {
	for source, target := range targets {
		if relatedDevicePaths(path, source) {
			return target, true
		}
	}
	return "", false
}

// relatedDevicePaths reports whether a and b name the same underlying block
// device — identical, or one names a partition of the other — checked in
// both directions since either side of a mounted-device comparison could be
// the whole device or a partition of it (e.g. /dev/sda mounted while
// /dev/sda1 is the candidate Path, or vice versa). A non-device path (e.g. a
// disk-image file backing a LUN) only ever matches by exact equality, since
// it has no partitions to relate.
func relatedDevicePaths(a, b string) bool {
	return a == b || isPartitionOf(a, b) || isPartitionOf(b, a)
}

// isPartitionOf reports whether child names a partition of the whole device
// parent, using Linux's device-partition naming convention: a parent whose
// name ends in a digit (nvme0n1, mmcblk0) numbers its partitions with a "p"
// separator (nvme0n1p1, mmcblk0p1) — otherwise a partition number would be
// indistinguishable from another whole device's name — while every other
// parent (sda, vda) appends the partition digit directly (sda1). Restricted
// to /dev paths: a disk-image file backing a LUN follows no such
// convention, so e.g. "/data/image.bin" and a same-directory
// "/data/image.bin2" must never be treated as device and partition.
func isPartitionOf(parent, child string) bool {
	if parent == "" || !strings.HasPrefix(parent, "/dev/") || !strings.HasPrefix(child, parent) {
		return false
	}
	suffix := strings.TrimPrefix(child, parent)
	if suffix == "" {
		return false
	}
	if parent[len(parent)-1] >= '0' && parent[len(parent)-1] <= '9' {
		rest, ok := strings.CutPrefix(suffix, "p")
		if !ok {
			return false
		}
		suffix = rest
	}
	if suffix == "" {
		return false
	}
	for _, r := range suffix {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
