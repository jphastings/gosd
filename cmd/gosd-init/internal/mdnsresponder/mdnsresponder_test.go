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

	waitFor(t, func() bool { return ns.callCount() == 1 }, "NewServer was never called")
	if !log.contains("answering as my-device.local") {
		t.Errorf("log missing start message: %v", log.snapshot())
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

func TestRunRetriesOnNextChangeAfterInitialFailure(t *testing.T) {
	ns := &fakeNewServer{}
	ns.script(serverResult{err: errBoom}, serverResult{srv: &fakeServer{}})
	log := &testLog{}
	changed := make(chan struct{})
	stop := make(chan struct{})
	defer close(stop)

	go Run(Deps{NewServer: ns.NewServer, Changed: changed, Log: log.Printf}, Options{Hostname: "my-device", Stop: stop})

	waitFor(t, func() bool { return ns.callCount() == 1 }, "initial NewServer call never happened")
	if !log.contains("will retry on the next network change") {
		t.Errorf("log missing initial-failure message: %v", log.snapshot())
	}

	changed <- struct{}{}

	waitFor(t, func() bool { return ns.callCount() == 2 }, "NewServer was not retried after a change notification")
	if !log.contains("answering as my-device.local") {
		t.Errorf("log missing eventual success message: %v", log.snapshot())
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

// Burst coalescing itself (N Notify calls before a receiver reads collapse
// to one pending item) is covered deterministically in signal_test.go, in
// isolation from any consumer. It can't be asserted at the Run level too:
// once a live receiver is draining the channel concurrently, whether N
// separate Notify calls collapse into one restart or several depends on
// how the scheduler interleaves them against the receiver — genuinely racy
// to observe from outside, not a bug in Run.
