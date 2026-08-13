//go:build !linux

// gosd-init only ever runs on Linux (it's PID 1 inside a Linux initramfs).
// This file exists purely so the boot package — and everything that
// imports it, including the pure orchestration logic tested with fakes —
// builds and `go test ./...` passes on macOS, per this repo's cross-platform
// testing requirement.
package boot

import (
	"errors"
	"io"
	"time"

	"github.com/jphastings/gosd/internal/redact"
)

var errUnsupportedPlatform = errors.New("gosd-init: not supported outside Linux")

// NewPlatform returns stub implementations that fail clearly if ever
// invoked. It exists only to keep this package buildable on non-Linux
// hosts; gosd-init itself is only ever built and run for linux/arm64.
func NewPlatform() *Platform {
	return &Platform{
		Mounter:               unsupportedPlatform{},
		Hostname:              unsupportedPlatform{},
		AppStarter:            unsupportedPlatform{},
		Reaper:                unsupportedPlatform{},
		Rebooter:              unsupportedPlatform{},
		OpenConsole:           func() (io.WriteCloser, error) { return nil, errUnsupportedPlatform },
		IgnoreShutdownSignals: func() {},
		WriteFatalReport:      func(string, string) error { return errUnsupportedPlatform },
		RemoveBootFiles:       func(string, []string) error { return errUnsupportedPlatform },
		EditBootPartition:     func(string, func(string) error) error { return errUnsupportedPlatform },
		DeviceModel:           func() string { return "" },
		Uptime:                func() (time.Duration, bool) { return 0, false },
		RegisteredSecrets:     func() []redact.Rule { return nil },
	}
}

type unsupportedPlatform struct{}

func (unsupportedPlatform) Mount(string, string, string, uintptr, string) error {
	return errUnsupportedPlatform
}

func (unsupportedPlatform) Unmount(string) error { return errUnsupportedPlatform }

func (unsupportedPlatform) SetHostname(string) error { return errUnsupportedPlatform }

func (unsupportedPlatform) Start(string, []string, io.Writer, io.Writer) (int, error) {
	return 0, errUnsupportedPlatform
}

func (unsupportedPlatform) Wait(int) (ExitStatus, error) { return ExitStatus{}, errUnsupportedPlatform }

func (unsupportedPlatform) Sync() {}

func (unsupportedPlatform) FlushConsole() {}

func (unsupportedPlatform) Reboot() {}

func (unsupportedPlatform) Halt() {}
