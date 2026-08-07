// Package childbackoff implements the exponential-backoff-with-cap restart
// delay shared by the gosd-init modules that supervise a single auxiliary
// child process — extracted from
// cmd/gosd-init/internal/cloudflared/backoff.go (bean gosd-wxjy) so a second
// gosd-init-supervised agent (epic gosd-65uy) can reuse the exact same
// doubling/capping engine instead of duplicating it. Bounds, and any
// "how long counts as stable" threshold, are each caller's own restart
// policy — passed into NewBackoff as base/max, and tracked by the caller
// itself for the stable-reset decision (see cloudflared.StableAfter) —
// this package only ever does the doubling and capping.
package childbackoff

import "time"

// Backoff computes the exponential restart delay for a crash-looping child
// process: it doubles on each consecutive call to Next, capped at max, and
// can be reset back to base once the caller decides the process has proven
// stable.
type Backoff struct {
	base, max time.Duration
	delay     time.Duration
}

// NewBackoff creates a Backoff that starts at base and never exceeds max —
// the caller's own restart policy, passed in rather than hardcoded so each
// gosd-init module using this package can pick its own bounds.
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
