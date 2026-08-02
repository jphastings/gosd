//go:build !linux

package gadget

import "errors"

// errUnsupportedPlatform is returned by MassStorage.Create's real
// mounted-device check off Linux. Apply only runs meaningfully on a GoSD
// board (configfs itself doesn't exist elsewhere) — this stub exists so the
// package builds and its logic tests run on the developer's macOS/other
// host, which drive Create through the writableFS fake and inject their own
// mountedTargets rather than reaching this default.
var errUnsupportedPlatform = errors.New("gadget: checking whether a MassStorage.Path is mounted is only supported on Linux boards")

func defaultMountedTargets() (map[string]string, error) {
	return nil, errUnsupportedPlatform
}
