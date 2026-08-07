//go:build linux

package disk

import (
	"github.com/jphastings/gosd/internal/blockmount"
	"github.com/jphastings/gosd/internal/diskfmt"
)

// newPlatformDeps wires the real disk operations. Inspect and Format come from
// internal/diskfmt (pure go-diskfs, no syscalls); the mount-state check and the
// mount itself are Linux syscalls/sysfs reads from internal/blockmount.
// Discover is replaced per call, since FormatAndMountDevice names its device.
// The last six fields are ext4-only (see blockmount.Deps): unused whenever
// FormatAndMountWith is called with FAT32 or exFAT, wired here regardless
// since disk (unlike emmc) can be asked for ext4.
func newPlatformDeps() blockmount.Deps {
	return blockmount.Deps{
		MountedAt:           blockmount.MountedAt,
		Discover:            discover,
		Inspect:             diskfmt.Inspect,
		Format:              diskfmt.Format,
		Mount:               blockmount.Mount,
		Mountable:           blockmount.Mountable,
		MountedSources:      blockmount.MountedSources,
		SyncDevice:          blockmount.SyncDevice,
		Grow:                blockmount.GrowEXT4,
		EstablishMarker:     blockmount.EstablishEXT4Marker,
		MarkerEstablished:   blockmount.EXT4MarkerEstablished,
		RootHasOtherContent: blockmount.RootHasOtherContent,
		Unmount:             blockmount.Unmount,
	}
}

// Unmount unmounts a filesystem FormatAndMount mounted, releasing its block
// device so another owner can take it exclusively — e.g. handing BlockDevice to
// gadget.MassStorage, which needs the raw device to itself (expose or mount,
// never both). Unmounting an already-unmounted mountpoint reports EINVAL, which
// callers switching modes can treat as already-released.
func Unmount(mountpoint string) error {
	return blockmount.Unmount(mountpoint)
}

// Devices lists the block devices FormatAndMount would consider, best first, so
// an app with more than one disk attached can pick with FormatAndMountDevice.
// The list holds only devices that pass every safety check — the right class,
// present medium, writable, and nothing mounted from them — so it is empty
// exactly when FormatAndMount would report ErrNoDisk.
func Devices() ([]string, error) {
	devices, mountedSources, err := survey()
	if err != nil {
		return nil, err
	}
	return candidates(devices, mountedSources), nil
}

func discover() (string, error) {
	devices, mountedSources, err := survey()
	if err != nil {
		return "", err
	}
	return choose(devices, mountedSources)
}

func verifyNamedDevice(device string) (string, error) {
	devices, mountedSources, err := survey()
	if err != nil {
		return "", err
	}
	return verifyNamed(device, devices, mountedSources)
}

func survey() ([]blockmount.Device, map[string]bool, error) {
	devices, err := blockmount.ReadBlockDevices()
	if err != nil {
		return nil, nil, err
	}
	mountedSources, err := blockmount.MountedSources()
	if err != nil {
		return nil, nil, err
	}
	return devices, mountedSources, nil
}
