package boot

import (
	"syscall"
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
		Wait:        func(int) (ExitStatus, error) { return ExitStatus{}, nil }, // exits immediately every time
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
		Wait: func(pid int) (ExitStatus, error) {
			if pid <= 2 {
				return ExitStatus{ExitCode: 1}, nil // crashes immediately
			}
			clock.Sleep(45 * time.Second) // this run is long enough to be "stable"
			return ExitStatus{}, nil
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
		Wait: func(int) (ExitStatus, error) {
			select {
			case <-stable:
			case <-time.After(2 * time.Second):
				t.Error("the app exited without a stable run ever being reported")
			}
			close(stop)
			return ExitStatus{}, nil
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
		Wait:  func(int) (ExitStatus, error) { return ExitStatus{ExitCode: 1}, nil }, // crashes immediately, every time
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
		Wait:        func(int) (ExitStatus, error) { return ExitStatus{}, nil },
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

// TestSupervisorReportsOnExitWithTheFullExitStatus pins that OnExit sees
// exactly what Wait returned, including signal detail Supervisor itself has
// no opinion about — deciding what counts as a crash is sequence.go's job,
// not this package's (gosd-s9uq).
func TestSupervisorReportsOnExitWithTheFullExitStatus(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	stop := make(chan struct{})
	starts := 0
	var got []ExitStatus

	sup := &Supervisor{
		Start: func() (int, error) {
			starts++
			return starts, nil
		},
		Wait: func(pid int) (ExitStatus, error) {
			if pid == 1 {
				return ExitStatus{ExitCode: 1}, nil
			}
			close(stop)
			return ExitStatus{Signaled: true, Signal: syscall.SIGSEGV, ExitCode: -1}, nil
		},
		Sleep:       func(time.Duration) {},
		Now:         clock.Now,
		Backoff:     NewBackoff(1*time.Second, 10*time.Second),
		StableAfter: 30 * time.Second,
		OnExit:      func(status ExitStatus, ran time.Duration) bool { got = append(got, status); return false },
		Log:         func(string, ...any) {},
	}

	sup.Run(stop)

	if len(got) != 2 {
		t.Fatalf("OnExit called %d times, want 2", len(got))
	}
	if got[0].ExitCode != 1 || got[0].Signaled {
		t.Errorf("first OnExit call = %+v, want a clean signal-free ExitCode 1", got[0])
	}
	if !got[1].Signaled || got[1].Signal != syscall.SIGSEGV {
		t.Errorf("second OnExit call = %+v, want Signaled with SIGSEGV", got[1])
	}
}

// TestSupervisorSkipsOnExitWhenWaitFails pins that a Wait error (never
// happens against the real reaper, but the interface allows it) leaves
// OnExit uncalled rather than handed a meaningless zero-value ExitStatus.
func TestSupervisorSkipsOnExitWhenWaitFails(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	stop := make(chan struct{})
	calls := 0

	sup := &Supervisor{
		Start: func() (int, error) {
			close(stop)
			return 1, nil
		},
		Wait:        func(int) (ExitStatus, error) { return ExitStatus{}, errBoom },
		Sleep:       func(time.Duration) {},
		Now:         clock.Now,
		Backoff:     NewBackoff(1*time.Second, 10*time.Second),
		StableAfter: 30 * time.Second,
		OnExit:      func(ExitStatus, time.Duration) bool { calls++; return false },
		Log:         func(string, ...any) {},
	}

	sup.Run(stop)

	if calls != 0 {
		t.Errorf("OnExit called %d times after a Wait error, want 0", calls)
	}
}
