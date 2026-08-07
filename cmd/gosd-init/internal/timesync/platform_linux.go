//go:build linux

package timesync

import (
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

// NewPlatform wires up the real NTP client, the settimeofday-backed
// SystemClock, and the ioctl-backed RTC.
func NewPlatform() *Platform {
	return &Platform{
		NTP:    newBeevikClient(),
		System: unixSystemClock{},
		RTC:    newUnixRTC(),
	}
}

// unixSystemClock implements SystemClock using settimeofday(2), the only
// way to set the running kernel's wall clock at all — on any board, RTC
// or none. A battery-backed RTC (see unixRTC) is written back to after
// every step this package applies, never read directly by this package:
// the kernel's own HCTOSYS copies it in at boot, before gosd-init starts.
type unixSystemClock struct{}

func (unixSystemClock) Set(t time.Time) error {
	tv := unix.NsecToTimeval(t.UnixNano())
	return unix.Settimeofday(&tv)
}

// rtcDevicePath is where a battery-backed RTC exposes itself on every
// board that has one (see gosd-achn): HYM8563 (nanopi-zero2), the
// RK808 family (rock-4se, radxa-zero-3e), SUN6I (cubie-a5e), and PL031
// (qemu-virt). The Pi family has no RTC at all, so this path simply never
// exists there.
const rtcDevicePath = "/dev/rtc0"

// unixRTC implements RTC using ioctl(RTC_SET_TIME) on rtcDevicePath.
// present is decided once, at newUnixRTC time (called once from
// NewPlatform, itself called once per gosd-init process) — never
// re-probed per sync — so a board with no RTC produces no per-sync log
// noise: os.Stat failing is exactly as expected, and exactly as boring,
// on every subsequent boot as it was the first time this ran.
type unixRTC struct {
	present bool
}

func newUnixRTC() unixRTC {
	_, err := os.Stat(rtcDevicePath)
	return unixRTC{present: err == nil}
}

func (r unixRTC) Set(t time.Time) error {
	if !r.present {
		return ErrRTCNotPresent
	}

	f, err := os.OpenFile(rtcDevicePath, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("opening %s: %w", rtcDevicePath, err)
	}
	defer func() { _ = f.Close() }()

	rt := toRTCTime(t)
	if err := unix.IoctlSetRTCTime(int(f.Fd()), &rt); err != nil {
		return fmt.Errorf("ioctl(RTC_SET_TIME) on %s: %w", rtcDevicePath, err)
	}
	return nil
}

// toRTCTime converts t (in UTC, matching how the kernel treats every RTC
// this package targets — see rtcDevicePath) into the wire struct
// RTC_SET_TIME expects. Wday, Yday, and Isdst are left zero: the kernel
// ignores all three on a set, only ever filling them in on a read.
func toRTCTime(t time.Time) unix.RTCTime {
	t = t.UTC()
	return unix.RTCTime{
		Sec:  int32(t.Second()),
		Min:  int32(t.Minute()),
		Hour: int32(t.Hour()),
		Mday: int32(t.Day()),
		Mon:  int32(t.Month() - 1), // struct rtc_time months are 0-11
		Year: int32(t.Year() - 1900),
	}
}
