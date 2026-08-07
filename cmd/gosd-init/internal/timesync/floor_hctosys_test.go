package timesync

import (
	"testing"
	"time"
)

// This file pins gosd-jyq8's audit finding: HCTOSYS (the kernel's own
// RTC-to-system-clock copy, CONFIG_RTC_HCTOSYS) runs before gosd-init even
// starts, and this package never reads the RTC or the pre-sync system
// clock at all — SystemClock and RTC (see interfaces.go) are Set-only, no
// Get, and the only two call sites of System.Set are stepClock (always
// with an NTP-derived newTime) and nowhere else in the whole module (grep
// for Settimeofday: platform_linux.go's unixSystemClock.Set is the only
// implementation). opts.Floor is therefore purely a validity gate
// (checkFloor, stepGuard.check's pre-floor-anchor fast path) on what NTP
// is allowed to report — it is never itself written to the system clock —
// so a system clock already correctly seeded by HCTOSYS from a
// battery-backed RTC can never be clobbered backward by the floor logic.
// These three tests correspond to the bean's three required scenarios.

// TestRunNeverSetsSystemClockBeforeFirstNTPSuccess models "RTC gave a time
// later than the build timestamp": since this package never reads
// whatever HCTOSYS already set the clock to, the only guarantee it can
// (and must) provide is that it never calls System.Set on its own
// initiative — only once a real NTP result arrives — and never with
// opts.Floor itself. A plausible, already-correct HCTOSYS-seeded clock is
// therefore left completely alone for as long as NTP hasn't answered yet.
func TestRunNeverSetsSystemClockBeforeFirstNTPSuccess(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	ntp := newFakeNTPClient()
	floor := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	syncedTime := floor.Add(200 * 24 * time.Hour) // a plausible, well-post-floor result
	ntp.script("ntp1", ntpResult{err: errBoom}, ntpResult{err: errBoom}, ntpResult{t: syncedTime})
	sys := &fakeSystemClock{}
	up := &flag{}
	up.set(true)
	log := &testLog{}
	deps, _ := newTestDeps(clock, ntp, sys, up, log)

	stop := make(chan struct{})
	defer close(stop)
	opts := defaultOptions([]string{"ntp1"}, stop)
	opts.Floor = floor

	go Run(deps, opts)

	for i := 0; i < 2; i++ {
		if !waitForPending(clock, 1) {
			t.Fatalf("no pending backoff timer after failed attempt %d", i+1)
		}
		if got := sys.sets(); len(got) != 0 {
			t.Fatalf("System.Set called %v before any NTP result had landed, want none (a valid pre-sync clock must be left alone)", got)
		}
		clock.Advance(10 * time.Second)
	}

	deadline := time.Now().Add(2 * time.Second)
	for len(sys.sets()) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	got := sys.sets()
	if len(got) != 1 || !got[0].Equal(syncedTime) {
		t.Fatalf("System.Set calls = %v, want exactly one call with the NTP result %v", got, syncedTime)
	}
	if got[0].Equal(floor) {
		t.Fatal("System.Set must never be called with the floor value itself — the floor is a validity check on NTP results, not a value ever applied to the clock")
	}
}

// TestRunFloorRefusesEarlyResultsRegardlessOfPresyncClock models "RTC gave
// an earlier-than-build time": whatever the pre-sync clock reads (this
// package never inspects it), an NTP result before the floor is always
// refused and retried rather than applied — floor semantics work exactly
// the same regardless of why the clock might currently be wrong. The
// eventual accepted result may be exactly the floor itself (not
// necessarily later): checkFloor only refuses results strictly before it.
func TestRunFloorRefusesEarlyResultsRegardlessOfPresyncClock(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	ntp := newFakeNTPClient()
	floor := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tooEarly := floor.Add(-24 * time.Hour) // e.g. a dead-battery RTC reset to some earlier date
	atFloor := floor                       // exactly the floor is a valid, accepted result
	ntp.script("ntp1", ntpResult{t: tooEarly}, ntpResult{t: atFloor})
	sys := &fakeSystemClock{}
	up := &flag{}
	up.set(true)
	log := &testLog{}
	deps, _ := newTestDeps(clock, ntp, sys, up, log)

	stop := make(chan struct{})
	defer close(stop)
	opts := defaultOptions([]string{"ntp1"}, stop)
	opts.Floor = floor

	go Run(deps, opts)

	if !waitForPending(clock, 1) {
		t.Fatal("no pending backoff timer after the pre-floor attempt")
	}
	if got := sys.sets(); len(got) != 0 {
		t.Fatalf("System.Set called %v for a pre-floor NTP result, want none", got)
	}
	if !log.contains("before this build's floor") {
		t.Errorf("log missing floor-refusal message: %v", log.snapshot())
	}

	clock.Advance(10 * time.Second) // exceeds any backoff delay scripted

	deadline := time.Now().Add(2 * time.Second)
	for len(sys.sets()) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	got := sys.sets()
	if len(got) != 1 || !got[0].Equal(atFloor) {
		t.Fatalf("System.Set calls = %v, want exactly one call with %v (the floor itself, the boundary case)", got, atFloor)
	}
}

// TestRunSystemClockBehaviorUnchangedWithNoRTCPresent models "no RTC" (the
// Pi family, gosd-achn): confirms RTC absence has zero bearing on the
// floor/first-sync path. The epoch-start clock a no-RTC board boots with
// is corrected by the first successful NTP sync exactly as it always has
// been — RTC presence or absence only ever affects the write-back
// gosd-lx8g added, never this read-side behavior.
func TestRunSystemClockBehaviorUnchangedWithNoRTCPresent(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0)) // epoch: what a boot with no RTC at all reads
	ntp := newFakeNTPClient()
	floor := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	syncedTime := floor.Add(48 * time.Hour)
	ntp.script("ntp1", ntpResult{t: syncedTime})
	sys := &fakeSystemClock{}
	up := &flag{}
	up.set(true)
	log := &testLog{}
	deps, _ := newTestDeps(clock, ntp, sys, up, log)
	deps.RTC = &fakeRTC{err: ErrRTCNotPresent} // no battery-backed RTC at all

	stop := make(chan struct{})
	defer close(stop)
	opts := defaultOptions([]string{"ntp1"}, stop)
	opts.Floor = floor

	go Run(deps, opts)

	deadline := time.Now().Add(2 * time.Second)
	for len(sys.sets()) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	got := sys.sets()
	if len(got) != 1 || !got[0].Equal(syncedTime) {
		t.Fatalf("System.Set calls = %v, want exactly one call with %v, unaffected by RTC absence", got, syncedTime)
	}
	if log.contains("RTC") {
		t.Errorf("RTC absence must stay silent and must not otherwise change sync behavior: %v", log.snapshot())
	}
}
