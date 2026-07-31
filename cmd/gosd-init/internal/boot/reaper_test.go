package boot

import (
	"testing"
	"time"
)

// waitFor calls r.Wait(pid) off the test goroutine so a lost status fails the
// test instead of hanging it forever — which is exactly what the bug being
// pinned here did to gosd-init's supervise loop.
func waitFor(t *testing.T, r *reaper, pid int) int {
	t.Helper()

	got := make(chan int, 1)
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
		return 0
	}
}

func TestWaitClaimsStatusReapedBeforeWaitWasCalled(t *testing.T) {
	r := newReaper()

	// /app exited within microseconds of exec (bad env, wrong-arch binary,
	// immediate os.Exit): the SIGCHLD drain reaps it before the supervisor
	// gets as far as calling Wait on the pid Start returned.
	r.deliver(4242, 3)

	if status := waitFor(t, r, 4242); status != 3 {
		t.Errorf("Wait returned status %d, want 3", status)
	}
}

func TestWaitReceivesStatusReapedWhileWaiting(t *testing.T) {
	r := newReaper()

	go func() {
		time.Sleep(10 * time.Millisecond)
		r.deliver(4242, 7)
	}()

	if status := waitFor(t, r, 4242); status != 7 {
		t.Errorf("Wait returned status %d, want 7", status)
	}
}

func TestStashSurvivesGrandchildReapsBeforeWait(t *testing.T) {
	r := newReaper()

	// gosd-init supervises one child at a time and calls Wait as soon as
	// Start returns, so an eviction that mattered would need a whole stash's
	// worth of orphaned grandchildren to be reaped inside that window. A
	// realistic burst leaves the app's status claimable.
	r.deliver(4242, 5)
	for pid := 5000; pid < 5000+maxStashedResults-1; pid++ {
		r.deliver(pid, 0)
	}

	if status := waitFor(t, r, 4242); status != 5 {
		t.Errorf("Wait returned status %d, want 5", status)
	}
}

func TestUnclaimedStatusesDoNotAccumulate(t *testing.T) {
	r := newReaper()

	// PID 1 lives for the life of the device, reaping grandchildren nobody
	// ever waits for; remembering them all would leak.
	for pid := 5000; pid < 5000+10*maxStashedResults; pid++ {
		r.deliver(pid, 0)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.results) > maxStashedResults {
		t.Errorf("stashed %d results, want at most %d", len(r.results), maxStashedResults)
	}
}
