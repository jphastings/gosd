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
