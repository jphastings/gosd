package timesync

import "time"

// NTPClient queries a single NTP server for the current, corrected time.
// Implementations must be safe to use from a single goroutine at a time
// (timesync never calls it concurrently).
type NTPClient interface {
	// Query performs an SNTP round-trip against server and returns the
	// sample it reports. It does not itself touch the system clock, and
	// it does not decide whether the sample is trustworthy — that's
	// validateSample's job, run by queryServers on every result, so the
	// decision is made in one place regardless of which NTPClient
	// implementation produced the sample (see fakeNTPClient).
	Query(server string) (SNTPSample, error)
}

// SNTPSample is the subset of an SNTP response timesync validates before
// ever trusting it (see validateSample and gosd-dqps). Real wire parsing
// belongs to the locked github.com/beevik/ntp dependency, not this
// package; ntpclient.go is the seam where its parsed *ntp.Response
// becomes this package's own type, which is what lets validateSample be
// exercised deterministically with fakeNTPClient instead of depending on
// vendor internals.
type SNTPSample struct {
	// Time is the corrected local time this sample implies: this
	// client's own clock plus the server's measured offset. This is what
	// gets applied to the system clock once a sample passes validation.
	Time time.Time

	// Leap is the server's leap-indicator field (RFC 5905 §7.3).
	// sntpLeapNotInSync (3, "alarm condition, clock not synchronized") is
	// exactly the state of a freshly booted router that hasn't reached
	// its own upstream time source yet — the expected bench case, not an
	// exotic attack.
	Leap uint8

	// Stratum is the server's NTP stratum. 0 is a Kiss-of-Death/control
	// response carrying no usable time; anything above 15 is invalid per
	// RFC 5905.
	Stratum uint8

	// TransmitTimestamp is the raw time the server says it sent this
	// reply at, straight off the wire — distinct from Time, which is
	// this client's own clock corrected by the measured offset. A server
	// with no notion of correct time at all (freshly booted, never
	// synced) is prone to reporting an era-zero TransmitTimestamp even
	// when its Leap and Stratum fields are otherwise well-formed.
	TransmitTimestamp time.Time
}

// sntpLeapNotInSync mirrors ntp.LeapNotInSync (RFC 5905's LI=3), spelled
// out as a plain constant here so validateSample and its tests don't
// need to import beevik/ntp just to compare Leap values — matching this
// package's existing convention of not leaking vendor types across the
// NTPClient boundary (see Clock's doc comment).
const sntpLeapNotInSync = 3

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
