//go:build !linux

package disk

import (
	"errors"

	"github.com/jphastings/gosd/internal/blockmount"
	"github.com/jphastings/gosd/internal/diskfmt"
)

// errUnsupportedPlatform is returned by the real disk operations off Linux.
// FormatAndMount only runs meaningfully on a GoSD board; these stubs exist so
// the package builds and its logic tests run on the developer's macOS/other
// host (which drive the pure orchestration and selection with fakes, not
// these).
var errUnsupportedPlatform = errors.New("disk: attached mass storage is only supported on Linux boards")

func newPlatformDeps() blockmount.Deps {
	return blockmount.Deps{
		MountedAt: func(string) (string, bool, error) { return "", false, errUnsupportedPlatform },
		Discover:  discover,
		Inspect:   diskfmt.Inspect,
		Format:    diskfmt.FormatFAT32,
		Mount:     func(string, string) error { return errUnsupportedPlatform },
	}
}

// Unmount is the off-Linux stub for the real unmount in platform_linux.go.
func Unmount(string) error { return errUnsupportedPlatform }

// Devices is the off-Linux stub for the real enumeration in platform_linux.go.
func Devices() ([]string, error) { return nil, errUnsupportedPlatform }

func discover() (string, error) { return "", errUnsupportedPlatform }

func verifyNamedDevice(string) (string, error) { return "", errUnsupportedPlatform }
