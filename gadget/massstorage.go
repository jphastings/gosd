package gadget

import "fmt"

// MassStorage is a USB mass-storage Function (configfs f_mass_storage): it
// exposes one LUN, backed by a block device or disk-image file on the
// board, as a removable-drive-style disk on the host. Requires the board's
// kernel to carry CONFIG_USB_CONFIGFS_MASS_STORAGE=y — see COMPATIBILITY.md's
// USB gadget footnote for per-board status.
//
// While the gadget is applied the host owns the backing store outright,
// caching and writing raw blocks with no coordination — the app must not
// mount or write Path itself at the same time: expose or mount, never both.
// A single LUN covers GoSD's use cases today; f_mass_storage itself supports
// additional lun.N directories, a possible future extension.
type MassStorage struct {
	// Path is the block device (e.g. /dev/nvme0n1p1) or disk-image file
	// backing the LUN. Required.
	Path string
	// ReadOnly write-protects the LUN: the host can read but not modify it.
	ReadOnly bool
	// Removable reports the medium as removable (like a USB thumb drive),
	// so the host offers a clean eject.
	Removable bool
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
