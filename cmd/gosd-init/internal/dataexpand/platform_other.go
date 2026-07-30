//go:build !linux

// gosd-init only ever runs on Linux; this file keeps the package — and its
// fake-driven pure logic tests — building on macOS, per this repo's
// cross-platform testing requirement.
package dataexpand

import (
	"errors"
	"time"

	"github.com/jphastings/gosd/internal/diskfmt"
)

var errUnsupportedPlatform = errors.New("gosd-init: not supported outside Linux")

// NewDeps returns stub implementations that fail clearly if ever invoked;
// gosd-init itself is only ever built and run for Linux.
func NewDeps(log func(format string, args ...any)) Deps {
	return Deps{
		ReadMBR:         func(string) ([]byte, error) { return nil, errUnsupportedPlatform },
		WriteMBR:        func(string, []byte) error { return errUnsupportedPlatform },
		DeviceSizeBytes: func(string) (int64, error) { return 0, errUnsupportedPlatform },
		AddKernelPartition: func(string, int, int64, int64) error {
			return errUnsupportedPlatform
		},
		Inspect:     func(string) (diskfmt.Contents, error) { return diskfmt.Contents{}, errUnsupportedPlatform },
		FormatFAT32: func(string, string) error { return errUnsupportedPlatform },
		PathExists:  func(string) bool { return false },
		Sleep:       time.Sleep,
		Now:         time.Now,
		Log:         log,
	}
}
