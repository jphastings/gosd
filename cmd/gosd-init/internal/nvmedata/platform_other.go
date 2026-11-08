//go:build !linux

package nvmedata

import (
	"errors"
	"time"

	"github.com/jphastings/gosd/internal/blockmount"
)

// NewDeps keeps the package building off Linux, where gosd-init never runs:
// Mount fails at its first step, listing the block devices.
func NewDeps(log func(format string, args ...any)) Deps {
	return Deps{
		Devices: func() ([]blockmount.Device, map[string]bool, error) {
			return nil, nil, errors.New("gosd-init: NVMe drives are only supported on Linux")
		},
		Sleep: time.Sleep,
		Log:   log,
	}
}
