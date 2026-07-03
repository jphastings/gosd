//go:build !linux

// gosd-init only ever runs on Linux (it's PID 1 inside a Linux initramfs).
// This file exists purely so the netup package — and everything that
// imports it, including the pure state machine tested with fakes — builds
// and `go test ./...` passes on macOS, per this repo's cross-platform
// testing requirement.
package netup

import (
	"context"
	"errors"
	"net"
)

var errUnsupportedPlatform = errors.New("gosd-init: networking not supported outside Linux")

// NewPlatform returns stub implementations that fail clearly if ever
// invoked. It exists only to keep this package buildable on non-Linux
// hosts; gosd-init itself is only ever built and run for linux/arm64.
func NewPlatform() *Platform {
	return &Platform{
		Links: unsupportedLinks{},
		DHCP:  unsupportedDHCP{},
	}
}

type unsupportedLinks struct{}

func (unsupportedLinks) SetUp(string) error              { return errUnsupportedPlatform }
func (unsupportedLinks) AddAddr(string, net.IPNet) error { return errUnsupportedPlatform }
func (unsupportedLinks) ReplaceDefaultRoute(string, net.IP) error {
	return errUnsupportedPlatform
}
func (unsupportedLinks) Watch(<-chan struct{}) (<-chan LinkEvent, error) {
	return nil, errUnsupportedPlatform
}

type unsupportedDHCP struct{}

func (unsupportedDHCP) Request(context.Context, string) (*Lease, error) {
	return nil, errUnsupportedPlatform
}

func (unsupportedDHCP) Renew(context.Context, string, *Lease) (*Lease, error) {
	return nil, errUnsupportedPlatform
}
