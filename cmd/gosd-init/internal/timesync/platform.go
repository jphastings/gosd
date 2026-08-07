package timesync

// Platform bundles the real implementations of NTPClient, SystemClock,
// and RTC. NewPlatform is implemented once per build tag
// (platform_linux.go, platform_other.go) so main.go can wire it up
// without caring which OS it's running on.
type Platform struct {
	NTP    NTPClient
	System SystemClock
	RTC    RTC
}
