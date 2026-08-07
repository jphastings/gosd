package cloudflared

import "time"

// Clock abstracts time so Run's gating and supervision loops can be driven
// deterministically in tests, without any real waiting.
//
// This is deliberately the same shape as netup.Clock and timesync.Clock,
// duplicated rather than imported or shared — see timesync.Clock's doc for
// why: each package only needs to know the paths and durations another
// package's Options/Deps hands it, never anything else about that package.
type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}
