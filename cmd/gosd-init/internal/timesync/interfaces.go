package timesync

import "time"

// NTPClient queries a single NTP server for the current, corrected time.
// Implementations must be safe to use from a single goroutine at a time
// (timesync never calls it concurrently).
type NTPClient interface {
	// Query performs an SNTP round-trip against server and returns the
	// current time, already corrected for network delay and the server's
	// clock offset. It does not itself touch the system clock.
	Query(server string) (time.Time, error)
}

// SystemClock sets the OS wall-clock time. Neither board has a
// battery-backed RTC, so this — settimeofday(2) on Linux — is the only
// way gosd-init ever gets a correct clock.
type SystemClock interface {
	Set(t time.Time) error
}

// Clock abstracts time so the retry/refresh state machine in timesync.go
// can be driven deterministically in tests, without any real waiting.
//
// This is deliberately the same shape as netup.Clock, duplicated rather
// than imported: timesync only needs to know the *path* netup writes
// (passed in via Options/Deps by main.go's wiring), not anything else
// about the networking packages, mirroring how boot.Backoff is its own
// independent copy rather than a shared dependency on netup.
type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}
