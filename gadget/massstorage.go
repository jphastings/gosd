package gadget

import (
	"errors"
	"fmt"

	"github.com/jphastings/gosd/internal/devreserve"
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
//
// # Devices GoSD keeps for itself
//
// Some of the board's storage is never the app's to publish, whatever the
// app intends: the boot partition carries the kernel this device starts
// from and the config tree that provisions it, so a host given write access
// to it gets code execution on the next boot. Create refuses those outright
// — wrapping [ErrReservedDevice], and regardless of whether anything is
// mounted at the time — for Path itself and for the whole disk a reserved
// partition sits on, since a LUN over the disk contains the partition.
//
// It learns which devices those are from gosd-init, which publishes them at
// boot (see the reserved-device list in
// [github.com/jphastings/gosd/internal/devreserve]); the refusal is
// therefore a rule the package enforces rather than, as it was until bean
// gosd-ix0r, an accident of gosd-init happening to keep /boot mounted. On
// an image whose gosd-init predates that list — or anywhere off a GoSD
// board — nothing is published, and Create falls back to the mounted-device
// check alone, exactly as it behaved before.
//
// The data partition is deliberately NOT among them: it is the app's own
// persistent storage, and examples/usbwebsite shares it behind an explicit
// operator opt-in on the eMMC-less boards. What lives on it that shouldn't
// be published is gosd-init's config store, which bean gosd-onjv moves to a
// partition of its own — and which, being reserved there, this refusal will
// cover.
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

	// reservedDevices reports the block devices gosd-init has claimed for
	// the board's own operation. nil (the zero value) means "read the real
	// /run file" (see defaultReservedDevices); tests override it directly,
	// the same way they do mountedTargets. Unlike the mount check this
	// needs no per-platform implementation: it reads one ordinary file at
	// a fixed path, which simply isn't there off a GoSD board.
	reservedDevices func() (devreserve.Reservations, error)
}

// ErrReservedDevice reports that MassStorage.Path names, or contains, a
// block device GoSD keeps for the board's own operation — today the boot
// partition, and so the whole disk holding it. Create's returned error
// wraps it, so an app that can do something else when it can't share a
// particular volume (offer a different one, or carry on without the drive)
// can match it with errors.Is and degrade gracefully, the same way
// [ErrNoController] already lets it.
var ErrReservedDevice = errors.New("the device is reserved for GoSD's own use")

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

	// Reservations first: they hold whether or not anything is mounted, so
	// a device that is both reserved and mounted deserves the error that
	// stays true after an Unmount rather than the one that invites it.
	if err := m.checkReserved(); err != nil {
		return err
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

// checkReserved refuses a Path that would hand a USB host one of the block
// devices gosd-init reserves for the board's own operation. See the type's
// "Devices GoSD keeps for itself" section for what is on the list and what
// deliberately isn't.
func (m MassStorage) checkReserved() error {
	reservedFn := m.reservedDevices
	if reservedFn == nil {
		reservedFn = defaultReservedDevices
	}
	reserved, err := reservedFn()
	if err != nil {
		return fmt.Errorf("MassStorage: reading the devices GoSD reserves (%s) failed, so it can't tell whether %s is one of them: %w — gosd-init writes that file at boot, so reboot the board; if something else on the device created it, remove it", devreserve.Path, m.Path, err)
	}

	entry, blocked := reserved.Exposes(m.Path)
	if !blocked {
		return nil
	}
	// Exposes already established that Path covers the reservation;
	// covering in the other direction too is what makes them the same
	// device rather than the disk one sits on. Asked this way rather than
	// by comparing the strings, so the two checks can't drift over what
	// counts as the same path.
	if devreserve.Covers(entry.Path, m.Path) {
		return fmt.Errorf("MassStorage.Path %s is %s, so %w, mounted or not: a LUN is the whole volume, so the host would get this board's kernel and every setting it was provisioned with — the WiFi passphrase and any ingress token in plain text among them — and without ReadOnly it could write files there that outlive the owner's next re-flash. Back the LUN with a volume your app owns outright instead: an eMMC or attached disk it formatted itself, or the SD card's data partition", m.Path, entry.Describe(), ErrReservedDevice)
	}
	return fmt.Errorf("MassStorage.Path %s is the whole disk holding %s (%s), so %w, mounted or not: a LUN is the whole volume, so sharing the disk shares that partition with it. Back the LUN with a single partition your app owns outright rather than the disk it sits on", m.Path, entry.Describe(), entry.Path, ErrReservedDevice)
}

// defaultReservedDevices is checkReserved's real source: the list gosd-init
// publishes on tmpfs at boot. A file that isn't there reports no
// reservations and no error — see devreserve's package doc for why absence
// degrades rather than refuses.
func defaultReservedDevices() (devreserve.Reservations, error) {
	return devreserve.Read(devreserve.Path)
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
//
// devreserve.Covers is the one-directional half of this — "would sharing
// the first hand over the second" — and the naming convention itself lives
// there, so the mounted-device check and the reserved-device check can
// never disagree about what a partition of what is.
func relatedDevicePaths(a, b string) bool {
	return devreserve.Covers(a, b) || devreserve.Covers(b, a)
}
