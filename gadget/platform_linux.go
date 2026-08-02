//go:build linux

package gadget

import "github.com/jphastings/gosd/internal/blockmount"

// defaultMountedTargets is MassStorage.Create's real mounted-device check —
// blockmount's own /proc/mounts reader, shared with the emmc and disk
// packages so the three can never disagree about what "mounted" means.
func defaultMountedTargets() (map[string]string, error) {
	return blockmount.MountedTargets()
}
