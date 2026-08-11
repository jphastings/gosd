package boot

import (
	"syscall"
	"testing"
	"time"
)

// waitFor calls r.Wait(pid) off the test goroutine so a lost status fails the
// test instead of hanging it forever — which is exactly what the bug being
// pinned here did to gosd-init's supervise loop.
func waitFor(t *testing.T, r *reaper, pid int) ExitStatus {
	t.Helper()

	got := make(chan ExitStatus, 1)
	go func() {
		status, err := r.Wait(pid)
		if err != nil {
			t.Errorf("Wait(%d) returned error: %v", pid, err)
		}
		got <- status
	}()

	select {
	case status := <-got:
		return status
	case <-time.After(2 * time.Second):
		t.Fatalf("Wait(%d) blocked; its exit status was lost", pid)
		return ExitStatus{}
	}
}

func TestWaitClaimsStatusReapedBeforeWaitWasCalled(t *testing.T) {
	r := newReaper()

	// /app exited within microseconds of exec (bad env, wrong-arch binary,
	// immediate os.Exit): the SIGCHLD drain reaps it before the supervisor
	// gets as far as calling Wait on the pid Start returned.
	r.deliver(4242, ExitStatus{ExitCode: 3})

	if status := waitFor(t, r, 4242); status.ExitCode != 3 {
		t.Errorf("Wait returned status %+v, want ExitCode 3", status)
	}
}

func TestWaitReceivesStatusReapedWhileWaiting(t *testing.T) {
	r := newReaper()

	go func() {
		time.Sleep(10 * time.Millisecond)
		r.deliver(4242, ExitStatus{ExitCode: 7})
	}()

	if status := waitFor(t, r, 4242); status.ExitCode != 7 {
		t.Errorf("Wait returned status %+v, want ExitCode 7", status)
	}
}

// TestWaitPreservesSignalDetail pins the whole reason ExitStatus exists
// (gosd-s9uq): a signal death has to survive the reaper round-trip intact,
// not just collapse to ExitStatus()'s -1.
func TestWaitPreservesSignalDetail(t *testing.T) {
	r := newReaper()

	r.deliver(4242, ExitStatus{ExitCode: -1, Signaled: true, Signal: syscall.SIGSEGV})

	status := waitFor(t, r, 4242)
	if !status.Signaled || status.Signal != syscall.SIGSEGV {
		t.Errorf("Wait returned status %+v, want Signaled with SIGSEGV", status)
	}
}

// TestConcurrentWaitersOnDistinctPidsBothResolve pins the reaper's amended
// stash comment (gosd-66ax, gosd-oyhi carve-out): gosd-init now supervises a
// small, fixed set of children — /app and, when baked, cloudflared — each
// with its own goroutine parked in Wait for its own pid at the same time.
// Both waiters are parked (registered in r.waiters) before either pid is
// delivered, so this genuinely exercises two concurrent Waiters, not two
// sequential ones; neither may block on, or steal the status meant for, the
// other's pid.
func TestConcurrentWaitersOnDistinctPidsBothResolve(t *testing.T) {
	r := newReaper()

	appDone := make(chan ExitStatus, 1)
	cloudflaredDone := make(chan ExitStatus, 1)

	go func() {
		status, err := r.Wait(4242)
		if err != nil {
			t.Errorf("Wait(4242) returned error: %v", err)
		}
		appDone <- status
	}()
	go func() {
		status, err := r.Wait(9999)
		if err != nil {
			t.Errorf("Wait(9999) returned error: %v", err)
		}
		cloudflaredDone <- status
	}()

	// Give both goroutines time to reach Wait's parked state before either
	// pid is reaped.
	time.Sleep(10 * time.Millisecond)

	r.deliver(9999, ExitStatus{ExitCode: 2})
	r.deliver(4242, ExitStatus{ExitCode: 1})

	select {
	case status := <-appDone:
		if status.ExitCode != 1 {
			t.Errorf("Wait(4242) returned status %+v, want ExitCode 1", status)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Wait(4242) blocked; its exit status was lost")
	}

	select {
	case status := <-cloudflaredDone:
		if status.ExitCode != 2 {
			t.Errorf("Wait(9999) returned status %+v, want ExitCode 2", status)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Wait(9999) blocked; its exit status was lost")
	}
}

func TestStashSurvivesGrandchildReapsBeforeWait(t *testing.T) {
	r := newReaper()

	// gosd-init calls Wait on a child as soon as its own Start returns, so
	// an eviction that mattered would need a whole stash's worth of
	// orphaned grandchildren to be reaped inside that one child's own
	// window. A realistic burst leaves the app's status claimable.
	r.deliver(4242, ExitStatus{ExitCode: 5})
	for pid := 5000; pid < 5000+maxStashedResults-1; pid++ {
		r.deliver(pid, ExitStatus{})
	}

	if status := waitFor(t, r, 4242); status.ExitCode != 5 {
		t.Errorf("Wait returned status %+v, want ExitCode 5", status)
	}
}

func TestUnclaimedStatusesDoNotAccumulate(t *testing.T) {
	r := newReaper()

	// PID 1 lives for the life of the device, reaping grandchildren nobody
	// ever waits for; remembering them all would leak.
	for pid := 5000; pid < 5000+10*maxStashedResults; pid++ {
		r.deliver(pid, ExitStatus{})
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.results) > maxStashedResults {
		t.Errorf("stashed %d results, want at most %d", len(r.results), maxStashedResults)
	}
}
