package netup

import "time"

// Bounds for RunDHCP's continued-failure status logging: the first failure
// in a streak always logs immediately (as before); after that, logging
// backs off the same way the retries themselves do, starting at
// minStatusInterval and doubling up to maxStatusInterval, so a board that
// never gets a lease keeps saying so without a line every retry (gosd-yx94:
// observed retries can be sub-second, which would otherwise spam the
// console for as long as discovery keeps failing).
const (
	minStatusInterval = 10 * time.Second
	maxStatusInterval = 5 * time.Minute
)

// retryStatus decides when a run of DHCP discovery failures is worth
// logging again. The first failure of a streak is always reported (kept
// as the same "failed: ...; retrying in ..." line as before); every
// failure after that is silent until the backing-off interval elapses, at
// which point one status line is logged naming how long discovery has
// been failing, and Reset (called on success) clears the streak so the
// next unrelated failure — e.g. after a later link flap — starts over.
type retryStatus struct {
	clock Clock

	inStreak bool
	since    time.Time
	interval time.Duration
	nextAt   time.Time
}

func newRetryStatus(clock Clock) *retryStatus {
	return &retryStatus{clock: clock}
}

// fail is called on every failed discovery attempt. It logs the immediate
// per-attempt message on the first failure of a streak; on later failures
// it logs a "still failing" status line only once the current backoff
// interval has elapsed, then doubles that interval (capped) for next time.
func (s *retryStatus) fail(deps Deps, iface string, err error, retryDelay time.Duration) {
	now := s.clock.Now()

	if !s.inStreak {
		s.inStreak = true
		s.since = now
		s.interval = minStatusInterval
		s.nextAt = now.Add(s.interval)
		deps.Log("DHCP discovery on %s failed: %v; retrying in %s", iface, err, retryDelay)
		return
	}

	if now.Before(s.nextAt) {
		return
	}

	deps.Log("DHCP discovery on %s is still failing after %s; last error: %v",
		iface, now.Sub(s.since).Round(time.Second), err)

	s.interval *= 2
	if s.interval > maxStatusInterval {
		s.interval = maxStatusInterval
	}
	s.nextAt = now.Add(s.interval)
}

// reset clears the failure streak, called once discovery succeeds so a
// later, unrelated failure (e.g. after a link flap) is reported as a fresh
// streak rather than continuing a stale one.
func (s *retryStatus) reset() {
	s.inStreak = false
}
