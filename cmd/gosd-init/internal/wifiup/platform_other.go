//go:build !linux

// gosd-init only ever runs on Linux (it's PID 1 inside a Linux
// initramfs). This file exists purely so the wifiup package — and
// everything that imports it, including the pure state machine tested
// with fakes — builds and `go test ./...` passes on macOS, per this
// repo's cross-platform testing requirement.
package wifiup

import "errors"

var errUnsupportedPlatform = errors.New("gosd-init: WiFi not supported outside Linux")

// NewPlatform returns an error unconditionally, so that on non-Linux
// hosts WiFi bring-up is treated exactly like "opening nl80211 failed" —
// gosd-init itself is only ever built and run for linux/arm64.
func NewPlatform() (WifiClient, error) {
	return nil, errUnsupportedPlatform
}
