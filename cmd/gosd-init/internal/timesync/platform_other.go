//go:build !linux

// gosd-init only ever runs on Linux (it's PID 1 inside a Linux
// initramfs). This file exists purely so the timesync package — and
// everything that imports it, including the pure state machine tested
// with fakes — builds and `go test ./...` passes on macOS, per this
// repo's cross-platform testing requirement.
package timesync

import (
	"errors"
	"time"
)

var errUnsupportedPlatform = errors.New("gosd-init: settimeofday not supported outside Linux")

// NewPlatform returns the real (portable) NTP client plus a SystemClock
// stub that fails clearly if ever invoked. It exists only to keep this
// package buildable on non-Linux hosts; gosd-init itself is only ever
// built and run for linux/arm64.
func NewPlatform() *Platform {
	return &Platform{
		NTP:    newBeevikClient(),
		System: unsupportedSystemClock{},
	}
}

type unsupportedSystemClock struct{}

func (unsupportedSystemClock) Set(time.Time) error { return errUnsupportedPlatform }
