//go:build linux

package emmc

import (
	"github.com/jphastings/gosd/internal/blockmount"
	"github.com/jphastings/gosd/internal/diskfmt"
)

// newPlatformDeps wires the real eMMC operations. inspect and format come from
// internal/diskfmt (pure go-diskfs, no syscalls); discovery, the mount-state
// check, and the mount itself are Linux syscalls/sysfs reads from
// internal/blockmount.
func newPlatformDeps() blockmount.Deps {
	return blockmount.Deps{
		MountedAt: blockmount.MountedAt,
		Discover:  discoverEMMC,
		Inspect:   diskfmt.Inspect,
		Format:    diskfmt.FormatFAT32,
		Mount:     blockmount.MountVFAT,
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
