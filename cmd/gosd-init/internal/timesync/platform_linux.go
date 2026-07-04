//go:build linux

package timesync

import (
	"time"

	"golang.org/x/sys/unix"
)

// NewPlatform wires up the real NTP client and the settimeofday-backed
// SystemClock.
func NewPlatform() *Platform {
	return &Platform{
		NTP:    newBeevikClient(),
		System: unixSystemClock{},
	}
}

// unixSystemClock implements SystemClock using settimeofday(2), the only
// way to set the wall clock on a board with no battery-backed RTC.
type unixSystemClock struct{}

func (unixSystemClock) Set(t time.Time) error {
	tv := unix.NsecToTimeval(t.UnixNano())
	return unix.Settimeofday(&tv)
}
