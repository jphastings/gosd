package timesync

import (
	"math/rand"
	"time"
)

// Bounds for the NTP sync retry backoff, used until the first sync
// succeeds. Capped well above netup's DHCP backoff (30s): unlike a DHCP
// server on the local segment, a public NTP pool can be slow or briefly
// unreachable, and there's no urgency to hammer it once the network
// itself is up — TLS calls in /app are expected to gate on (or simply
// retry past) /run/gosd/time-synced not existing yet.
const (
	DefaultBackoffBase = 1 * time.Second
	DefaultBackoffCap  = 5 * time.Minute
)

// Backoff computes NTP sync retry delays: exponential, capped, with full
// jitter (a random delay in [0, computed delay], the AWS "full-jitter"
// strategy) so that a fleet of devices booting back up together after an
// outage doesn't all hit the same NTP servers in lockstep.
//
// This mirrors netup.Backoff exactly; duplicated rather than imported
// (see Clock's doc comment for why).
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

// Next returns the jittered delay to wait before the next sync attempt,
// and advances the underlying (pre-jitter) delay for the following call.
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
		// against the NTP server instead of backing off at all.
		return b.base
	}
	return jittered
}

// Reset returns the backoff to its initial state, used once a sync
// succeeds so the next unrelated failure starts from base again rather
// than continuing a stale sequence.
func (b *Backoff) Reset() {
	b.delay = 0
}
