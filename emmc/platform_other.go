//go:build !linux

package emmc

import (
	"errors"

	"github.com/jphastings/gosd/internal/emmcfmt"
)

// errUnsupportedPlatform is returned by the real eMMC operations off Linux.
// FormatAndMount only runs meaningfully on a GoSD board; these stubs exist so
// the package builds and its logic tests run on the developer's macOS/other
// host (which drive the pure orchestration with fakes, not these).
var errUnsupportedPlatform = errors.New("emmc: onboard eMMC is only supported on Linux boards")

func newPlatformDeps() deps {
	return deps{
		mountedAt: func(string) (bool, error) { return false, errUnsupportedPlatform },
		discover:  func() (string, error) { return "", errUnsupportedPlatform },
		inspect:   emmcfmt.Inspect,
		format:    emmcfmt.FormatFAT32,
		mount:     func(string, string) error { return errUnsupportedPlatform },
	}
}
