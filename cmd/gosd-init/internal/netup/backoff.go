package netup

import (
	"math/rand"
	"time"
)

// Bounds for the DHCP discovery retry backoff. The bean calls for
// "retries + jittered backoff forever" (the cable may be plugged in late,
// so discovery must keep retrying indefinitely), capped so a device that's
// simply never going to see a cable doesn't back off to some
// impractically long interval.
const (
	DefaultBackoffBase = 1 * time.Second
	DefaultBackoffCap  = 30 * time.Second
)

// Backoff computes DHCP discovery retry delays: exponential, capped, with
// full jitter (a random delay in [0, computed delay], the AWS
// "full-jitter" strategy) so that many devices retrying discovery at once
// (e.g. a fleet powering back on together after an outage) don't all hit
// the DHCP server in lockstep.
type Backoff struct {
	base, max time.Duration
	delay     time.Duration

	// random returns a value in [0, 1); overridden in tests for
	// deterministic assertions.
	random func() float64
}

// NewBackoff creates a Backoff that starts at base, doubles on every
// consecutive call to Next, and never proposes a delay longer than max.
func NewBackoff(base, max time.Duration) *Backoff {
	return &Backoff{base: base, max: max, random: rand.Float64}
}

// Next returns the jittered delay to wait before the next discovery
// attempt, and advances the underlying (pre-jitter) delay for the
// following call.
func (b *Backoff) Next() time.Duration {
	if b.delay <= 0 {
		b.delay = b.base
	} else {
		b.delay *= 2
		if b.delay > b.max {
			b.delay = b.max
		}
	}
	jittered := time.Duration(b.random() * float64(b.delay))
	if jittered <= 0 {
		// Never propose a zero-length wait: that would busy-loop
		// against the DHCP server instead of backing off at all.
		return b.base
	}
	return jittered
}

// Reset returns the backoff to its initial state, used once discovery
// succeeds so the next unrelated failure (e.g. after a later link flap)
// starts from base again rather than continuing a stale sequence.
func (b *Backoff) Reset() {
	b.delay = 0
}
