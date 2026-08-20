package mdnsresponder

import (
	"errors"
	"testing"
	"time"
)

var errBoom = errors.New("boom")

func waitFor(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal(msg)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestRunStartsResponderOnceAtStartup(t *testing.T) {
	ns := &fakeNewServer{}
	ns.script(serverResult{srv: &fakeServer{}})
	log := &testLog{}
	changed := make(chan struct{})
	stop := make(chan struct{})
	defer close(stop)

	go Run(Deps{NewServer: ns.NewServer, Changed: changed, Log: log.Printf}, Options{Hostname: "my-device", Stop: stop})

	// Wait on the log line itself, not on ns.callCount() reaching 1 followed
	// by an unguarded check of the log: restart() calls NewServer (which
	// bumps callCount) and only then logs, as two separate statements, so
	// polling callCount and immediately checking the log without waiting on
	// it races against that gap. Under CI scheduler contention that race is
	// observable (bean gosd-f352) - waiting on the actual condition removes
	// it entirely rather than papering over it with a longer timeout.
	waitFor(t, func() bool { return log.contains("answering as my-device.local") }, "responder never logged starting up")
	if got := ns.callCount(); got != 1 {
		t.Errorf("NewServer called %d times, want 1", got)
	}
}

func TestRunRestartsAndClosesPreviousResponderOnChange(t *testing.T) {
	first := &fakeServer{}
	second := &fakeServer{}
	ns := &fakeNewServer{}
	ns.script(serverResult{srv: first}, serverResult{srv: second})
	log := &testLog{}
	changed := make(chan struct{})
	stop := make(chan struct{})
	defer close(stop)

	go Run(Deps{NewServer: ns.NewServer, Changed: changed, Log: log.Printf}, Options{Hostname: "my-device", Stop: stop})

	waitFor(t, func() bool { return ns.callCount() == 1 }, "initial NewServer call never happened")

	changed <- struct{}{}

	waitFor(t, func() bool { return ns.callCount() == 2 }, "NewServer was not called again after a change notification")
	waitFor(t, func() bool { return first.closeCount() == 1 }, "previous responder was never closed on restart")
	if second.closeCount() != 0 {
		t.Errorf("the new responder was closed too, want only the old one: %d", second.closeCount())
	}
}

// TestRunRetriesOnNextChangeAfterInitialFailure was timing-flaky on macOS CI
// runners (bean gosd-f352). The apparent culprit was minRestartInterval's
// 250ms floor, but the real race was in the test itself: it polled
// ns.callCount() reaching a target value and then, with no further
// synchronization, immediately checked a log line that restart() only
// writes in the statement AFTER the one that bumps callCount. Nothing
// enforces a happens-before between "callCount observed" and "the log line
// exists" - under CI scheduler contention (most likely right as the
// minRestartInterval-gated retry's goroutine wakes from its timer, which is
// exactly the ~0.25s mark the flake was observed at) the log check could
// run before restart() reached its Log call. Waiting on the log line
// itself - the actual condition each assertion cares about - removes the
// race rather than widening a timeout or retrying.
func TestRunRetriesOnNextChangeAfterInitialFailure(t *testing.T) {
	ns := &fakeNewServer{}
	ns.script(serverResult{err: errBoom}, serverResult{srv: &fakeServer{}})
	log := &testLog{}
	changed := make(chan struct{})
	stop := make(chan struct{})
	defer close(stop)

	go Run(Deps{NewServer: ns.NewServer, Changed: changed, Log: log.Printf}, Options{Hostname: "my-device", Stop: stop})

	waitFor(t, func() bool { return log.contains("will retry on the next network change") }, "initial failure was never logged")
	if got := ns.callCount(); got != 1 {
		t.Errorf("NewServer called %d times before the first change notification, want 1", got)
	}

	changed <- struct{}{}

	waitFor(t, func() bool { return log.contains("answering as my-device.local") }, "responder never came up after being retried")
	if got := ns.callCount(); got != 2 {
		t.Errorf("NewServer called %d times after a change notification, want 2", got)
	}
}

func TestRunClosesCurrentResponderWhenStopped(t *testing.T) {
	srv := &fakeServer{}
	ns := &fakeNewServer{}
	ns.script(serverResult{srv: srv})
	log := &testLog{}
	changed := make(chan struct{})
	stop := make(chan struct{})

	done := make(chan struct{})
	go func() {
		Run(Deps{NewServer: ns.NewServer, Changed: changed, Log: log.Printf}, Options{Hostname: "my-device", Stop: stop})
		close(done)
	}()

	waitFor(t, func() bool { return ns.callCount() == 1 }, "NewServer was never called")
	close(stop)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after Stop was closed")
	}
	if srv.closeCount() != 1 {
		t.Errorf("responder Close calls = %d, want 1", srv.closeCount())
	}
}

func TestRunRateLimitsRestartsUnderRapidChange(t *testing.T) {
	ns := &fakeNewServer{}
	ns.script(serverResult{srv: &fakeServer{}})
	log := &testLog{}
	changed := make(chan struct{})
	stop := make(chan struct{})
	defer close(stop)

	go Run(Deps{NewServer: ns.NewServer, Changed: changed, Log: log.Printf}, Options{Hostname: "my-device", Stop: stop})

	waitFor(t, func() bool { return ns.callCount() == 1 }, "initial NewServer call never happened")

	start := time.Now()
	for i := 0; i < 3; i++ {
		changed <- struct{}{}
	}

	deadline := time.Now().Add(3 * time.Second)
	for ns.callCount() != 4 {
		if time.Now().After(deadline) {
			t.Fatalf("NewServer was called %d times within 3s, want 4", ns.callCount())
		}
		time.Sleep(time.Millisecond)
	}

	// A rate-limited loop can't finish 3 restarts any faster than
	// 3*minRestartInterval; an unbounded loop would finish in microseconds.
	// Comparing against 2*minRestartInterval leaves comfortable margin for
	// scheduler jitter on either side of that line.
	if elapsed := time.Since(start); elapsed < 2*minRestartInterval {
		t.Errorf("3 change notifications produced 3 restarts in %s; want at least %s — minRestartInterval should have paced them", elapsed, 2*minRestartInterval)
	}
}

func TestRunStopsPromptlyDuringRateLimitWait(t *testing.T) {
	ns := &fakeNewServer{}
	ns.script(serverResult{srv: &fakeServer{}})
	log := &testLog{}
	changed := make(chan struct{})
	stop := make(chan struct{})

	done := make(chan struct{})
	go func() {
		Run(Deps{NewServer: ns.NewServer, Changed: changed, Log: log.Printf}, Options{Hostname: "my-device", Stop: stop})
		close(done)
	}()

	waitFor(t, func() bool { return ns.callCount() == 1 }, "initial NewServer call never happened")
	changed <- struct{}{}
	close(stop)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return promptly when Stop closed during the rate-limit wait")
	}
}

// Burst coalescing itself (N Notify calls before a receiver reads collapse
// to one pending item) is covered deterministically in signal_test.go, in
// isolation from any consumer. It can't be asserted at the Run level too:
// once a live receiver is draining the channel concurrently, whether N
// separate Notify calls collapse into one restart or several depends on
// how the scheduler interleaves them against the receiver — genuinely racy
// to observe from outside, not a bug in Run.
