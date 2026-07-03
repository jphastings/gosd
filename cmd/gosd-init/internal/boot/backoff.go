package boot

import "time"

// Boot-sequence-mandated restart backoff bounds: restart /app on exit with
// exponential backoff capped at 10s.
const (
	DefaultBackoffBase = 1 * time.Second
	DefaultBackoffCap  = 10 * time.Second

	// StableRunThreshold is how long /app must run before a subsequent exit
	// is treated as a fresh failure rather than a continuation of a crash
	// loop, resetting the backoff delay back to DefaultBackoffBase. This
	// isn't spelled out by the boot sequence itself; it's a deliberate,
	// commonly-used interpretation (systemd, Kubernetes) of "exponential
	// backoff" so that a device which crash-loops once early on doesn't
	// stay slow to restart for the rest of its uptime.
	StableRunThreshold = 30 * time.Second
)

// Backoff computes the exponential restart delay for a crash-looping child
// process: it doubles on each consecutive failure, capped at max, and can be
// reset back to base once the process has proven stable.
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
