package netup

import (
	"testing"
	"time"
)

func statusTestDeps(log *testLog) Deps {
	return Deps{Log: log.Printf}
}

func TestRetryStatusLogsFirstFailureImmediately(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	log := &testLog{}
	s := newRetryStatus(clock)

	s.fail(statusTestDeps(log), "eth0", errBoom, time.Second)

	if got := len(log.snapshot()); got != 1 {
		t.Fatalf("log has %d lines after first failure, want 1: %v", got, log.snapshot())
	}
	if !log.contains("DHCP discovery on eth0 failed") {
		t.Errorf("missing immediate failure line: %v", log.snapshot())
	}
}

// TestRetryStatusSuppressesRapidRepeats proves the fix for the "spams the
// console every few hundred milliseconds" half of gosd-yx94: many failures
// in quick succession (as the observed sub-second jittered retries were)
// must not each produce a log line.
func TestRetryStatusSuppressesRapidRepeats(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	log := &testLog{}
	deps := statusTestDeps(log)
	s := newRetryStatus(clock)

	for i := 0; i < 20; i++ {
		s.fail(deps, "eth0", errBoom, 300*time.Millisecond)
		clock.Advance(300 * time.Millisecond) // 6s total, well under minStatusInterval
	}

	if got := len(log.snapshot()); got != 1 {
		t.Fatalf("log has %d lines after 20 rapid failures within %s, want 1 (just the first): %v",
			got, minStatusInterval, log.snapshot())
	}
}

// TestRetryStatusReportsAgainAfterInterval proves the other half: a streak
// that keeps failing must still say so periodically, not go silent forever.
func TestRetryStatusReportsAgainAfterInterval(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	log := &testLog{}
	deps := statusTestDeps(log)
	s := newRetryStatus(clock)

	s.fail(deps, "eth0", errBoom, time.Second) // 1 line: the immediate failure

	clock.Advance(minStatusInterval)
	s.fail(deps, "eth0", errBoom, time.Second) // interval elapsed: 1 more line

	if got := len(log.snapshot()); got != 2 {
		t.Fatalf("log has %d lines, want 2 (initial + one status report): %v", got, log.snapshot())
	}
	if !log.contains("still failing after") {
		t.Errorf("status line missing elapsed-time wording: %v", log.snapshot())
	}
}

// TestRetryStatusIntervalBacksOff proves the reporting cadence itself backs
// off (rather than firing at a fixed period forever) up to the cap.
func TestRetryStatusIntervalBacksOff(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	log := &testLog{}
	deps := statusTestDeps(log)
	s := newRetryStatus(clock)

	s.fail(deps, "eth0", errBoom, time.Second) // immediate: interval now minStatusInterval

	// Advancing by exactly minStatusInterval a second time should NOT be
	// enough to report again, since the interval doubled after the first
	// status report... but no status report has happened yet, so the very
	// next elapse at minStatusInterval does report (case above). This test
	// checks the interval AFTER that first status report has doubled.
	clock.Advance(minStatusInterval)
	s.fail(deps, "eth0", errBoom, time.Second) // 2nd line, interval -> 2*minStatusInterval

	clock.Advance(minStatusInterval) // only 1x the (now doubled) interval
	s.fail(deps, "eth0", errBoom, time.Second)
	if got := len(log.snapshot()); got != 2 {
		t.Fatalf("log has %d lines after a sub-interval advance, want 2 (interval should have doubled): %v",
			got, log.snapshot())
	}

	clock.Advance(minStatusInterval) // now 2x total since the doubled interval started: due
	s.fail(deps, "eth0", errBoom, time.Second)
	if got := len(log.snapshot()); got != 3 {
		t.Fatalf("log has %d lines, want 3 (the doubled interval has now elapsed): %v", got, log.snapshot())
	}
}

func TestRetryStatusIntervalCapsAtMax(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	log := &testLog{}
	deps := statusTestDeps(log)
	s := newRetryStatus(clock)

	s.fail(deps, "eth0", errBoom, time.Second)
	for i := 0; i < 10; i++ {
		clock.Advance(s.interval)
		s.fail(deps, "eth0", errBoom, time.Second)
	}

	if s.interval != maxStatusInterval {
		t.Fatalf("interval = %s after repeated doublings, want capped at %s", s.interval, maxStatusInterval)
	}
}

func TestRetryStatusResetStartsNewStreak(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	log := &testLog{}
	deps := statusTestDeps(log)
	s := newRetryStatus(clock)

	s.fail(deps, "eth0", errBoom, time.Second)
	s.reset()
	s.fail(deps, "eth0", errBoom, time.Second) // treated as a fresh streak's first failure

	if got := len(log.snapshot()); got != 2 {
		t.Fatalf("log has %d lines, want 2 (both treated as immediate first failures): %v", got, log.snapshot())
	}
}
