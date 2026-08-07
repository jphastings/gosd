//go:build !linux

package emmc

import (
	"errors"

	"github.com/jphastings/gosd/internal/blockmount"
	"github.com/jphastings/gosd/internal/diskfmt"
)

// errUnsupportedPlatform is returned by the real eMMC operations off Linux.
// FormatAndMount only runs meaningfully on a GoSD board; these stubs exist so
// the package builds and its logic tests run on the developer's macOS/other
// host (which drive the pure orchestration with fakes, not these).
var errUnsupportedPlatform = errors.New("emmc: onboard eMMC is only supported on Linux boards")

func newPlatformDeps() blockmount.Deps {
	return blockmount.Deps{
		MountedAt:           func(string) (string, bool, error) { return "", false, errUnsupportedPlatform },
		Discover:            func() (string, error) { return "", errUnsupportedPlatform },
		Inspect:             diskfmt.Inspect,
		Format:              diskfmt.Format,
		Mount:               func(string, string, diskfmt.FS) error { return errUnsupportedPlatform },
		Mountable:           func(diskfmt.FS) (bool, error) { return false, errUnsupportedPlatform },
		MountedSources:      func() (map[string]bool, error) { return nil, errUnsupportedPlatform },
		SyncDevice:          func(string) error { return errUnsupportedPlatform },
		Grow:                func(string, string) error { return errUnsupportedPlatform },
		EstablishMarker:     func(string) error { return errUnsupportedPlatform },
		MarkerEstablished:   func(string) (bool, error) { return false, errUnsupportedPlatform },
		RootHasOtherContent: func(string) (bool, error) { return false, errUnsupportedPlatform },
		Unmount:             func(string) error { return errUnsupportedPlatform },
	}
}

// Unmount is the off-Linux stub for the real unmount in platform_linux.go.
func Unmount(string) error { return errUnsupportedPlatform }
