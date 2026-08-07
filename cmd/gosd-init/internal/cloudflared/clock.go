package cloudflared

import "time"

// realClock is the production Clock: real wall time, real timers. Mirrors
// netup.realClock and timesync.realClock exactly — duplicated rather than
// imported, per Clock's doc comment on why this package shares no types
// with its neighbors.
type realClock struct{}

// NewRealClock returns the production Clock implementation.
func NewRealClock() Clock { return realClock{} }

func (realClock) Now() time.Time { return time.Now() }

func (realClock) After(d time.Duration) <-chan time.Time { return time.After(d) }
