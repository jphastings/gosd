package cloudflared

import "time"

// Bounds and reset threshold for the cloudflared restart backoff (locked
// decision, bean gosd-uj36): a single auxiliary process, not the boot
// sequence's own /app — capped low enough (30s) that a permanently broken
// tunnel logs no more than ~2 lines/minute to the 115200 serial console,
// with no jitter needed since cloudflared is the only thing gosd-init ever
// starts here (jitter exists to spread a fleet's simultaneous retries
// across a shared resource, e.g. netup.Backoff's DHCP server; cloudflared
// itself already jitters its own edge reconnects internally).
const (
	DefaultBackoffBase = 1 * time.Second
	DefaultBackoffCap  = 30 * time.Second

	// StableAfter is how long cloudflared must run before its next exit
	// resets Backoff back to DefaultBackoffBase, mirroring boot.Supervisor's
	// StableRunThreshold (same rationale: a device that crash-loops once
	// early on shouldn't stay slow to restart for the rest of its uptime).
	StableAfter = 30 * time.Second
)

// Backoff computes the exponential restart delay for cloudflared exiting
// unexpectedly: it doubles on each consecutive call to Next, capped at max,
// and can be reset back to base once the process has proven stable.
//
// This mirrors boot.Backoff exactly (same doubling-with-cap shape, no
// jitter); duplicated rather than imported, per Clock's doc comment on why
// this package doesn't share types with its neighbors.
type Backoff struct {
	base, max time.Duration
	delay     time.Duration
}

// NewBackoff creates a Backoff that starts at base and never exceeds max.
func NewBackoff(base, max time.Duration) *Backoff {
	return &Backoff{base: base, max: max}
}

// Next returns the delay to wait before the next restart attempt, doubling
// the delay (capped at max) for the following call.
func (b *Backoff) Next() time.Duration {
	if b.delay <= 0 {
		b.delay = b.base
	} else {
		b.delay *= 2
		if b.delay > b.max {
			b.delay = b.max
		}
	}
	return b.delay
}

// Reset returns the backoff to its initial state.
func (b *Backoff) Reset() {
	b.delay = 0
}
