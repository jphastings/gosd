package timesync

import (
	"errors"
	"time"
)

// rtcWriteback tracks whether an RTC write failure has already been
// logged for the lifetime of one Run call, so a persistently failing
// write (a flaky or half-dead RTC chip) produces a single boot-log line
// rather than one per sync for as long as the device stays up — mirroring
// how Run's "floor is disabled" line logs once, not per attempt. One
// instance is created per Run call and threaded through the first sync
// and every resync after it, exactly like stepGuard (see Run).
type rtcWriteback struct {
	warned bool
}

// apply writes newTime to deps.RTC immediately after a clock step this
// package has just applied (see stepClock — never called for a step that
// was refused or failed to apply). ErrRTCNotPresent, the expected case on
// a board with no battery-backed RTC at all, is never logged; any other
// error is logged at most once per Run call and otherwise ignored — the
// sync loop's own correctness never depends on the RTC write succeeding.
func (w *rtcWriteback) apply(deps Deps, newTime time.Time) {
	err := deps.RTC.Set(newTime)
	if err == nil || errors.Is(err, ErrRTCNotPresent) || w.warned {
		return
	}
	deps.Log("writing system time to the RTC failed: %v", err)
	w.warned = true
}
