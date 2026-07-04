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
	for len(sys.sets()) == 0 && time.Now().Before(deadline) {
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
	for len(sys.sets()) == 0 && time.Now().Before(deadline) {
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
	for len(sys.sets()) != 1 && time.Now().Before(deadline) {
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
