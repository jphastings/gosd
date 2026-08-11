package boot

import (
	"testing"
	"time"
)

func TestSupervisorRestartsWithEscalatingBackoff(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	var sleeps []time.Duration
	stop := make(chan struct{})
	starts := 0

	sup := &Supervisor{
		Start: func() (int, error) {
			starts++
			if starts == 3 {
				close(stop)
			}
			return starts, nil
		},
		Wait:        func(int) (int, error) { return 0, nil }, // exits immediately every time
		Sleep:       func(d time.Duration) { sleeps = append(sleeps, d) },
		Now:         clock.Now,
		Backoff:     NewBackoff(1*time.Second, 10*time.Second),
		StableAfter: 30 * time.Second,
		Log:         func(string, ...any) {},
	}

	sup.Run(stop)

	if starts != 3 {
		t.Fatalf("Start called %d times, want 3", starts)
	}
	want := []time.Duration{1 * time.Second, 2 * time.Second}
	if len(sleeps) != len(want) {
		t.Fatalf("Sleep calls = %v, want %v", sleeps, want)
	}
	for i, w := range want {
		if sleeps[i] != w {
			t.Errorf("sleep %d = %s, want %s", i, sleeps[i], w)
		}
	}
}

func TestSupervisorResetsBackoffAfterAStableRun(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	var sleeps []time.Duration
	stop := make(chan struct{})
	starts := 0

	sup := &Supervisor{
		Start: func() (int, error) {
			starts++
			if starts == 4 {
				close(stop)
			}
			return starts, nil
		},
		Wait: func(pid int) (int, error) {
			if pid <= 2 {
				return 1, nil // crashes immediately
			}
			clock.Sleep(45 * time.Second) // this run is long enough to be "stable"
			return 0, nil
		},
		Sleep:       func(d time.Duration) { sleeps = append(sleeps, d) },
		Now:         clock.Now,
		Backoff:     NewBackoff(1*time.Second, 10*time.Second),
		StableAfter: 30 * time.Second,
		Log:         func(string, ...any) {},
	}

	sup.Run(stop)

	want := []time.Duration{1 * time.Second, 2 * time.Second, 1 * time.Second}
	if len(sleeps) != len(want) {
		t.Fatalf("Sleep calls = %v, want %v", sleeps, want)
	}
	for i, w := range want {
		if sleeps[i] != w {
			t.Errorf("sleep %d = %s, want %s", i, sleeps[i], w)
		}
	}
}

func TestSupervisorReportsAStableRunWhileTheAppIsStillRunning(t *testing.T) {
	// The healthy device is the one whose app never exits, so a stable run
	// has to be announced by a timer rather than waiting for an exit that
	// isn't coming — it's what tells the crash reporter the device has
	// recovered.
	clock := newFakeClock(time.Unix(0, 0))
	stop := make(chan struct{})
	stableAfter := make(chan time.Time, 1)
	stable := make(chan struct{})

	sup := &Supervisor{
		Start: func() (int, error) {
			stableAfter <- time.Unix(0, 0)
			return 1, nil
		},
		Wait: func(int) (int, error) {
			select {
			case <-stable:
			case <-time.After(2 * time.Second):
				t.Error("the app exited without a stable run ever being reported")
			}
			close(stop)
			return 0, nil
		},
		Sleep:       func(time.Duration) {},
		Now:         clock.Now,
		After:       func(time.Duration) <-chan time.Time { return stableAfter },
		Backoff:     NewBackoff(1*time.Second, 10*time.Second),
		StableAfter: 30 * time.Second,
		OnStableRun: func() { close(stable) },
		Log:         func(string, ...any) {},
	}

	sup.Run(stop)
}

func TestSupervisorDoesNotReportAStableRunForACrashLoop(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	stop := make(chan struct{})
	starts := 0
	stableRuns := 0

	sup := &Supervisor{
		Start: func() (int, error) {
			starts++
			if starts == 3 {
				close(stop)
			}
			return starts, nil
		},
		Wait:  func(int) (int, error) { return 1, nil }, // crashes immediately, every time
		Sleep: func(time.Duration) {},
		Now:   clock.Now,
		// A timer that never fires: the app never lives long enough.
		After:       func(time.Duration) <-chan time.Time { return make(chan time.Time) },
		Backoff:     NewBackoff(1*time.Second, 10*time.Second),
		StableAfter: 30 * time.Second,
		OnStableRun: func() { stableRuns++ },
		Log:         func(string, ...any) {},
	}

	sup.Run(stop)

	if stableRuns != 0 {
		t.Errorf("reported %d stable runs during a crash loop, want none", stableRuns)
	}
}

func TestSupervisorLogsStartFailuresAndKeepsRetrying(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	attempts := 0
	stop := make(chan struct{})

	sup := &Supervisor{
		Start: func() (int, error) {
			attempts++
			if attempts == 2 {
				close(stop)
			}
			return 0, errBoom
		},
		Wait:        func(int) (int, error) { return 0, nil },
		Sleep:       func(time.Duration) {},
		Now:         clock.Now,
		Backoff:     NewBackoff(1*time.Second, 10*time.Second),
		StableAfter: 30 * time.Second,
		Log:         func(string, ...any) {},
	}

	sup.Run(stop)

	if attempts != 2 {
		t.Fatalf("Start called %d times, want 2 (supervisor should keep retrying after a start failure)", attempts)
	}
}
