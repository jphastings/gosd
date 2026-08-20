//go:build linux

package emmc

import (
	"github.com/jphastings/gosd/internal/blockmount"
	"github.com/jphastings/gosd/internal/diskfmt"
)

// newPlatformDeps wires the real eMMC operations. inspect and format come from
// internal/diskfmt (pure go-diskfs, no syscalls); discovery, the mount-state
// check, the mount itself and the whole establishment sequence (sync, marker,
// unmount) are Linux syscalls/sysfs reads from internal/blockmount. Only Grow
// is ext4-only (see blockmount.Deps); everything else runs for every
// filesystem emmc can be asked for.
func newPlatformDeps() blockmount.Deps {
	return blockmount.Deps{
		MountedAt:           blockmount.MountedAt,
		Discover:            discoverEMMC,
		Inspect:             diskfmt.Inspect,
		Format:              diskfmt.Format,
		Mount:               blockmount.Mount,
		Mountable:           blockmount.Mountable,
		MountedSources:      blockmount.MountedSources,
		SyncDevice:          blockmount.SyncDevice,
		Grow:                blockmount.GrowEXT4,
		EstablishMarker:     blockmount.EstablishMarker,
		MarkerEstablished:   blockmount.MarkerEstablished,
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

func discoverEMMC() (string, error) {
	devices, err := blockmount.ReadBlockDevices()
	if err != nil {
		return "", err
	}
	mountedSources, err := blockmount.MountedSources()
	if err != nil {
		return "", err
	}
	return chooseEMMC(devices, mountedSources)
}
