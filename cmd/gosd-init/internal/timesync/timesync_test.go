package timesync

import (
	"errors"
	"testing"
	"time"
)

var errBoom = errors.New("boom")

func newTestDeps(clock *fakeClock, ntp *fakeNTPClient, sys *fakeSystemClock, up *flag, log *testLog) (Deps, *counter) {
	marked := &counter{}
	deps := Deps{
		NTP:        ntp,
		System:     sys,
		Clock:      clock,
		NewBackoff: func() *Backoff { return noJitterBackoff(time.Second, 10*time.Second) },
		NetworkUp: func() (bool, error) {
			return up.get(), nil
		},
		MarkTimeSynced: func() error {
			marked.inc()
			return nil
		},
		Log: log.Printf,
	}
	return deps, marked
}

func defaultOptions(servers []string, stop <-chan struct{}) Options {
	return Options{
		Servers:               servers,
		ResyncEvery:           time.Hour,
		NetworkUpPollInterval: 2 * time.Second,
		Stop:                  stop,
	}
}

func TestRunWaitsForNetworkUpBeforeQuerying(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	ntp := newFakeNTPClient()
	syncedTime := time.Unix(1700000000, 0)
	ntp.script("ntp1", ntpResult{t: syncedTime})
	sys := &fakeSystemClock{}
	up := &flag{}
	log := &testLog{}
	deps, marked := newTestDeps(clock, ntp, sys, up, log)

	stop := make(chan struct{})
	defer close(stop)
	go Run(deps, defaultOptions([]string{"ntp1"}, stop))

	if !waitForPending(clock, 1) {
		t.Fatal("timesync never registered the network-up poll timer")
	}
	if got := ntp.callCount("ntp1"); got != 0 {
		t.Fatalf("NTP queried %d times before network was up, want 0", got)
	}

	up.set(true)
	clock.Advance(2 * time.Second)

	deadline := time.Now().Add(2 * time.Second)
	for (len(sys.sets()) == 0 || marked.load() == 0) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := sys.sets(); len(got) != 1 || !got[0].Equal(syncedTime) {
		t.Fatalf("System.Set calls = %v, want exactly one call with %v", got, syncedTime)
	}
	if marked.load() != 1 {
		t.Errorf("time-synced marker written %d times, want 1", marked.load())
	}
}

func TestRunStopsWaitingForNetworkUpWhenStopClosed(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	ntp := newFakeNTPClient()
	sys := &fakeSystemClock{}
	up := &flag{} // never goes up
	log := &testLog{}
	deps, _ := newTestDeps(clock, ntp, sys, up, log)

	stop := make(chan struct{})
	close(stop)

	done := make(chan struct{})
	go func() {
		Run(deps, defaultOptions([]string{"ntp1"}, stop))
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after Stop was closed")
	}
	if got := ntp.callCount("ntp1"); got != 0 {
		t.Errorf("NTP queried %d times, want 0 (network never came up)", got)
	}
}

func TestRunRetriesWithBackoffUntilFirstSuccess(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	ntp := newFakeNTPClient()
	syncedTime := time.Unix(1700000000, 0)
	ntp.script("ntp1",
		ntpResult{err: errBoom},
		ntpResult{err: errBoom},
		ntpResult{t: syncedTime},
	)
	sys := &fakeSystemClock{}
	up := &flag{}
	up.set(true)
	log := &testLog{}
	deps, marked := newTestDeps(clock, ntp, sys, up, log)

	stop := make(chan struct{})
	defer close(stop)
	go Run(deps, defaultOptions([]string{"ntp1"}, stop))

	// Two failed rounds, each followed by a backoff wait.
	for i := 0; i < 2; i++ {
		deadline := time.Now().Add(2 * time.Second)
		for ntp.callCount("ntp1") != i+1 && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		if !waitForPending(clock, 1) {
			t.Fatalf("no pending backoff timer after failed attempt %d", i+1)
		}
		clock.Advance(10 * time.Second) // exceeds any backoff delay scripted
	}

	deadline := time.Now().Add(2 * time.Second)
	for (len(sys.sets()) == 0 || marked.load() == 0) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := sys.sets(); len(got) != 1 || !got[0].Equal(syncedTime) {
		t.Fatalf("System.Set calls = %v, want exactly one call with %v", got, syncedTime)
	}
	if marked.load() != 1 {
		t.Errorf("time-synced marker written %d times, want 1", marked.load())
	}
	if !log.contains("retrying in") {
		t.Errorf("log missing retry message: %v", log.snapshot())
	}
	if !log.contains("system clock synchronized") {
		t.Errorf("log missing step-change message: %v", log.snapshot())
	}
}

func TestRunTriesNextServerBeforeBackingOff(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	ntp := newFakeNTPClient()
	syncedTime := time.Unix(1700000000, 0)
	ntp.script("bad", ntpResult{err: errBoom})
	ntp.script("good", ntpResult{t: syncedTime})
	sys := &fakeSystemClock{}
	up := &flag{}
	up.set(true)
	log := &testLog{}
	deps, _ := newTestDeps(clock, ntp, sys, up, log)

	stop := make(chan struct{})
	defer close(stop)
	go Run(deps, defaultOptions([]string{"bad", "good"}, stop))

	deadline := time.Now().Add(2 * time.Second)
	for len(sys.sets()) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := sys.sets(); len(got) != 1 || !got[0].Equal(syncedTime) {
		t.Fatalf("System.Set calls = %v, want exactly one call with %v", got, syncedTime)
	}
	if ntp.callCount("bad") != 1 || ntp.callCount("good") != 1 {
		t.Errorf("callCount(bad)=%d, callCount(good)=%d, want 1, 1", ntp.callCount("bad"), ntp.callCount("good"))
	}
	if log.contains("retrying in") {
		t.Error("should not have backed off: the second server answered within the same round")
	}
}

func TestRunResyncsAfterInterval(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	ntp := newFakeNTPClient()
	first := time.Unix(1700000000, 0)
	second := time.Unix(1700003700, 0) // ~1h5m later, as a real resync would report
	ntp.script("ntp1", ntpResult{t: first}, ntpResult{t: second})
	sys := &fakeSystemClock{}
	up := &flag{}
	up.set(true)
	log := &testLog{}
	deps, marked := newTestDeps(clock, ntp, sys, up, log)

	stop := make(chan struct{})
	defer close(stop)
	go Run(deps, defaultOptions([]string{"ntp1"}, stop))

	deadline := time.Now().Add(2 * time.Second)
	for (len(sys.sets()) != 1 || marked.load() == 0) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(sys.sets()) != 1 {
		t.Fatalf("first sync never landed: sets=%v", sys.sets())
	}
	if marked.load() != 1 {
		t.Fatalf("time-synced marker written %d times, want 1", marked.load())
	}

	if !waitForPending(clock, 1) {
		t.Fatal("resync timer was never registered")
	}
	clock.Advance(time.Hour)

	deadline = time.Now().Add(2 * time.Second)
	for len(sys.sets()) != 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	got := sys.sets()
	if len(got) != 2 || !got[1].Equal(second) {
		t.Fatalf("System.Set calls = %v, want a second call with %v", got, second)
	}
	// The marker is only ever written once, on the first success.
	if marked.load() != 1 {
		t.Errorf("time-synced marker written %d times after resync, want still 1", marked.load())
	}
}

// TestRunRefusesPreFloorResultAndRetries is gosd-0esw's floor test: a
// forged (or badly misbehaving) server reporting a time before the image
// was ever built must be refused and logged, not applied — and, since
// that's indistinguishable from any other round where no server could be
// trusted, must fall straight into the normal backoff retry rather than
// wedging. The marker must not appear until a result actually clears the
// floor.
func TestRunRefusesPreFloorResultAndRetries(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	ntp := newFakeNTPClient()
	floor := time.Unix(1700000000, 0)
	tooEarly := floor.Add(-time.Hour)
	validTime := floor.Add(time.Hour)
	ntp.script("ntp1", ntpResult{t: tooEarly}, ntpResult{t: validTime})
	sys := &fakeSystemClock{}
	up := &flag{}
	up.set(true)
	log := &testLog{}
	deps, marked := newTestDeps(clock, ntp, sys, up, log)

	stop := make(chan struct{})
	defer close(stop)
	opts := defaultOptions([]string{"ntp1"}, stop)
	opts.Floor = floor

	go Run(deps, opts)

	if !waitForPending(clock, 1) {
		t.Fatal("no pending backoff timer after the pre-floor attempt")
	}
	if got := sys.sets(); len(got) != 0 {
		t.Fatalf("System.Set called %v after a pre-floor result, want none", got)
	}
	if marked.load() != 0 {
		t.Errorf("time-synced marker written %d times after a pre-floor result, want 0", marked.load())
	}
	if !log.contains("before this build's floor") {
		t.Errorf("log missing floor-refusal message: %v", log.snapshot())
	}

	clock.Advance(10 * time.Second) // exceeds any backoff delay scripted

	deadline := time.Now().Add(2 * time.Second)
	for (len(sys.sets()) == 0 || marked.load() == 0) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := sys.sets(); len(got) != 1 || !got[0].Equal(validTime) {
		t.Fatalf("System.Set calls = %v, want exactly one call with %v", got, validTime)
	}
	if marked.load() != 1 {
		t.Errorf("time-synced marker written %d times, want 1", marked.load())
	}
}

// TestRunAppliesOrdinaryResyncStepImmediately confirms a resync well
// within MaxStep is unaffected by the guard: applied on the very first
// try, no confirmation round needed.
func TestRunAppliesOrdinaryResyncStepImmediately(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	ntp := newFakeNTPClient()
	first := time.Unix(1700000000, 0)
	second := first.Add(time.Hour + 30*time.Second) // a normal clock-drift correction
	ntp.script("ntp1", ntpResult{t: first}, ntpResult{t: second})
	sys := &fakeSystemClock{}
	up := &flag{}
	up.set(true)
	log := &testLog{}
	deps, _ := newTestDeps(clock, ntp, sys, up, log)

	stop := make(chan struct{})
	defer close(stop)
	opts := defaultOptions([]string{"ntp1"}, stop)
	opts.MaxStep = 1000 * time.Second

	go Run(deps, opts)

	deadline := time.Now().Add(2 * time.Second)
	for len(sys.sets()) != 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(sys.sets()) != 1 {
		t.Fatalf("first sync never landed: sets=%v", sys.sets())
	}

	if !waitForPending(clock, 1) {
		t.Fatal("resync timer was never registered")
	}
	clock.Advance(time.Hour)

	deadline = time.Now().Add(2 * time.Second)
	for len(sys.sets()) != 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	got := sys.sets()
	if len(got) != 2 || !got[1].Equal(second) {
		t.Fatalf("System.Set calls = %v, want a second call with %v applied immediately", got, second)
	}
	if log.contains("max-step threshold") {
		t.Error("an ordinary resync step should never trip the max-step guard")
	}
}

// TestRunRefusesOverThresholdStepUntilSecondQueryAgrees is gosd-0esw's
// step-guard test: a long-powered-off device's first real resync reports
// a time far ahead of the (still wrong) system clock. That's refused and
// logged rather than stepped outright — but since the true offset stays
// essentially constant, the very next scheduled resync reports a
// consistent value and the guard accepts it, so a genuinely legitimate
// large step isn't wedged forever.
func TestRunRefusesOverThresholdStepUntilSecondQueryAgrees(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	ntp := newFakeNTPClient()
	first := time.Unix(1700000000, 0)
	// The true time is ~2 days ahead of the device's clock. Two
	// consecutive resyncs, an hour (opts.ResyncEvery) apart, both report
	// a time consistent with that same offset.
	bigJump1 := first.Add(48 * time.Hour)
	bigJump2 := bigJump1.Add(time.Hour)
	ntp.script("ntp1", ntpResult{t: first}, ntpResult{t: bigJump1}, ntpResult{t: bigJump2})
	sys := &fakeSystemClock{}
	up := &flag{}
	up.set(true)
	log := &testLog{}
	deps, _ := newTestDeps(clock, ntp, sys, up, log)

	stop := make(chan struct{})
	defer close(stop)
	opts := defaultOptions([]string{"ntp1"}, stop)
	opts.MaxStep = 1000 * time.Second

	go Run(deps, opts)

	// First sync lands unconditionally: there's no baseline yet to
	// step-guard against.
	deadline := time.Now().Add(2 * time.Second)
	for len(sys.sets()) != 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(sys.sets()) != 1 {
		t.Fatalf("first sync never landed: sets=%v", sys.sets())
	}

	if !waitForPending(clock, 1) {
		t.Fatal("first resync timer was never registered")
	}
	clock.Advance(time.Hour) // first scheduled resync: bigJump1

	deadline = time.Now().Add(2 * time.Second)
	for !log.contains("max-step threshold") && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !log.contains("max-step threshold") {
		t.Fatalf("log missing over-threshold refusal message: %v", log.snapshot())
	}
	if got := sys.sets(); len(got) != 1 {
		t.Fatalf("System.Set calls = %v, want still exactly 1 after the first over-threshold refusal", got)
	}

	if !waitForPending(clock, 1) {
		t.Fatal("second resync timer was never registered")
	}
	clock.Advance(time.Hour) // second scheduled resync: bigJump2, agrees with bigJump1

	deadline = time.Now().Add(2 * time.Second)
	for len(sys.sets()) != 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	got := sys.sets()
	if len(got) != 2 || !got[1].Equal(bigJump2) {
		t.Fatalf("System.Set calls = %v, want a second call with %v once the confirming query agreed", got, bigJump2)
	}
	if !log.contains("confirmed by a second") {
		t.Errorf("log missing step-confirmation message: %v", log.snapshot())
	}
}

// TestRunRefusesOverThresholdStepAgainWhenSecondQueryDisagrees confirms
// the guard doesn't just rubber-stamp any second over-threshold reading:
// one that doesn't roughly agree with the first is refused again (kept as
// the new pending candidate for whatever queries next), not treated as a
// confirmation.
func TestRunRefusesOverThresholdStepAgainWhenSecondQueryDisagrees(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	ntp := newFakeNTPClient()
	first := time.Unix(1700000000, 0)
	bigJump1 := first.Add(48 * time.Hour)
	// An hour has passed, but this candidate is nowhere near
	// bigJump1+1h: two independent forged/erratic readings, not one
	// consistent story.
	bigJump2 := bigJump1.Add(30 * 24 * time.Hour)
	ntp.script("ntp1", ntpResult{t: first}, ntpResult{t: bigJump1}, ntpResult{t: bigJump2})
	sys := &fakeSystemClock{}
	up := &flag{}
	up.set(true)
	log := &testLog{}
	deps, _ := newTestDeps(clock, ntp, sys, up, log)

	stop := make(chan struct{})
	defer close(stop)
	opts := defaultOptions([]string{"ntp1"}, stop)
	opts.MaxStep = 1000 * time.Second

	go Run(deps, opts)

	deadline := time.Now().Add(2 * time.Second)
	for len(sys.sets()) != 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !waitForPending(clock, 1) {
		t.Fatal("first resync timer was never registered")
	}
	clock.Advance(time.Hour) // bigJump1: refused, becomes pending

	deadline = time.Now().Add(2 * time.Second)
	for !log.contains("max-step threshold") && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}

	if !waitForPending(clock, 1) {
		t.Fatal("second resync timer was never registered")
	}
	clock.Advance(time.Hour) // bigJump2: still over threshold, disagrees with bigJump1

	// Wait for the second refusal to be logged (proof this resync ran
	// to completion) rather than a fixed sleep, then check nothing was
	// applied.
	deadline = time.Now().Add(2 * time.Second)
	for log.count("max-step threshold") < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if n := log.count("max-step threshold"); n != 2 {
		t.Fatalf("expected two over-threshold refusal log lines, got %d: %v", n, log.snapshot())
	}
	if got := sys.sets(); len(got) != 1 {
		t.Fatalf("System.Set calls = %v, a disagreeing second over-threshold reading must not be applied", got)
	}
}

// TestRunLogsOnceWhenFloorIsDisabled is gosd-dqps's "floor must never be
// silently absent" test: a zero Options.Floor (a config.json baked
// before the build-timestamp field existed, or a build that failed to
// bake one) must be visible in the boot log exactly once, not left as a
// silent gap the way JP's field report found it.
func TestRunLogsOnceWhenFloorIsDisabled(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	ntp := newFakeNTPClient()
	ntp.script("ntp1", ntpResult{t: time.Unix(1700000000, 0)})
	sys := &fakeSystemClock{}
	up := &flag{}
	up.set(true)
	log := &testLog{}
	deps, _ := newTestDeps(clock, ntp, sys, up, log)

	stop := make(chan struct{})
	defer close(stop)
	opts := defaultOptions([]string{"ntp1"}, stop) // Floor left zero

	go Run(deps, opts)

	deadline := time.Now().Add(2 * time.Second)
	for len(sys.sets()) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if n := log.count("floor is disabled"); n != 1 {
		t.Errorf("floor-disabled boot line logged %d times, want exactly 1: %v", n, log.snapshot())
	}
}

// TestRunDoesNotLogFloorDisabledWhenFloorIsSet confirms the boot line is
// specific to the disabled case: a properly baked Floor logs nothing
// about it being disabled.
func TestRunDoesNotLogFloorDisabledWhenFloorIsSet(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	ntp := newFakeNTPClient()
	floor := time.Unix(1700000000, 0)
	ntp.script("ntp1", ntpResult{t: floor.Add(time.Hour)})
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

	deadline := time.Now().Add(2 * time.Second)
	for len(sys.sets()) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if log.contains("floor is disabled") {
		t.Errorf("floor-disabled boot line logged despite a set Floor: %v", log.snapshot())
	}
}

// TestRunSchedulesConfirmingResyncSoonerThanResyncEvery is gosd-dqps's
// bounded-wrong-clock-time test: once a resync leaves a step-guard
// confirmation pending, Run must not wait the full ResyncEvery for the
// confirming query — advancing only DefaultPendingConfirmDelay (well
// short of the test's much longer ResyncEvery) must be enough to trigger
// it.
func TestRunSchedulesConfirmingResyncSoonerThanResyncEvery(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	ntp := newFakeNTPClient()
	first := time.Unix(1700000000, 0)
	bigJump1 := first.Add(48 * time.Hour)
	// The confirming query lands ~DefaultPendingConfirmDelay of real
	// (fake Clock) time after bigJump1, not a further ResyncEvery later
	// — that's the whole point of this test — so it must agree with a
	// candidate that moved by roughly that same amount, not by a full
	// hour as a same-ResyncEvery-apart pair would.
	bigJump2 := bigJump1.Add(DefaultPendingConfirmDelay + time.Second)
	ntp.script("ntp1", ntpResult{t: first}, ntpResult{t: bigJump1}, ntpResult{t: bigJump2})
	sys := &fakeSystemClock{}
	up := &flag{}
	up.set(true)
	log := &testLog{}
	deps, _ := newTestDeps(clock, ntp, sys, up, log)

	stop := make(chan struct{})
	defer close(stop)
	opts := defaultOptions([]string{"ntp1"}, stop)
	opts.ResyncEvery = 24 * time.Hour // deliberately far longer than the confirm delay
	opts.MaxStep = 1000 * time.Second

	go Run(deps, opts)

	deadline := time.Now().Add(2 * time.Second)
	for len(sys.sets()) != 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !waitForPending(clock, 1) {
		t.Fatal("first resync timer was never registered")
	}
	clock.Advance(opts.ResyncEvery) // the first scheduled resync: bigJump1, refused, becomes pending

	deadline = time.Now().Add(2 * time.Second)
	for !log.contains("max-step threshold") && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !waitForPending(clock, 1) {
		t.Fatal("confirming resync timer was never registered")
	}

	// Advancing only a bit past the (much shorter) pending-confirm delay
	// must be enough to fire the confirming query — nowhere near another
	// full ResyncEvery.
	clock.Advance(DefaultPendingConfirmDelay + time.Second)

	deadline = time.Now().Add(2 * time.Second)
	for len(sys.sets()) != 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	got := sys.sets()
	if len(got) != 2 || !got[1].Equal(bigJump2) {
		t.Fatalf("System.Set calls = %v, want a second call with %v well before a full ResyncEvery had elapsed", got, bigJump2)
	}
}

func TestRunLogsFailedResyncButKeepsGoing(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	ntp := newFakeNTPClient()
	first := time.Unix(1700000000, 0)
	ntp.script("ntp1", ntpResult{t: first}, ntpResult{err: errBoom})
	sys := &fakeSystemClock{}
	up := &flag{}
	up.set(true)
	log := &testLog{}
	deps, _ := newTestDeps(clock, ntp, sys, up, log)

	stop := make(chan struct{})
	defer close(stop)
	go Run(deps, defaultOptions([]string{"ntp1"}, stop))

	deadline := time.Now().Add(2 * time.Second)
	for len(sys.sets()) != 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(sys.sets()) != 1 {
		t.Fatalf("first sync never landed: sets=%v", sys.sets())
	}

	if !waitForPending(clock, 1) {
		t.Fatal("resync timer was never registered")
	}
	clock.Advance(time.Hour)

	deadline = time.Now().Add(2 * time.Second)
	for !log.contains("scheduled NTP resync failed") && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !log.contains("scheduled NTP resync failed") {
		t.Errorf("log missing failed-resync message: %v", log.snapshot())
	}
	// A failed resync must not add another System.Set call.
	if len(sys.sets()) != 1 {
		t.Errorf("System.Set calls = %v, want still exactly 1 after a failed resync", sys.sets())
	}
}
