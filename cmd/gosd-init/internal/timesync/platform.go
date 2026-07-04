package timesync

// Platform bundles the real implementations of NTPClient and SystemClock.
// NewPlatform is implemented once per build tag (platform_linux.go,
// platform_other.go) so main.go can wire it up without caring which OS
// it's running on.
type Platform struct {
	NTP    NTPClient
	System SystemClock
}
