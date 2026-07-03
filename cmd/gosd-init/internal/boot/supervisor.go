package boot

import "time"

// Supervisor runs /app, restarting it with exponential backoff whenever it
// exits, for as long as PID 1 lives. Every dependency is injected so the
// restart/backoff decisions can be unit-tested without real processes,
// clocks, or sleeps.
type Supervisor struct {
	// Start launches /app and returns its pid.
	Start func() (pid int, err error)
	// Wait blocks until pid has exited, returning its exit status.
	Wait func(pid int) (status int, err error)
	// Sleep pauses for the given duration between restart attempts.
	Sleep func(time.Duration)
	// Now returns the current time, used to measure how long /app ran.
	Now func() time.Time
	// Backoff computes the delay before each restart attempt.
	Backoff *Backoff
	// StableAfter is how long /app must run before its next exit resets
	// Backoff back to its base delay.
	StableAfter time.Duration
	// Log records what the supervisor is doing.
	Log func(format string, args ...any)
}

// Run starts and supervises /app until stop is closed (or, with a nil stop
// channel, forever — the normal PID 1 case, since gosd-init never
// gracefully shuts down).
func (s *Supervisor) Run(stop <-chan struct{}) {
	for {
		if stopped(stop) {
			return
		}

		s.runOnce()

		if stopped(stop) {
			return
		}
		s.Sleep(s.Backoff.Next())
	}
}

// stopped reports whether stop has been closed, without blocking.
func stopped(stop <-chan struct{}) bool {
	select {
	case <-stop:
		return true
	default:
		return false
	}
}

// runOnce starts /app, waits for it to exit, and resets the backoff if it
// ran long enough to be considered stable.
func (s *Supervisor) runOnce() {
	startedAt := s.Now()

	pid, err := s.Start()
	if err != nil {
		s.Log("starting /app failed: %v", err)
		return
	}
	s.Log("started /app (pid %d)", pid)

	status, err := s.Wait(pid)
	ran := s.Now().Sub(startedAt)
	if err != nil {
		s.Log("supervising /app (pid %d) failed: %v", pid, err)
	} else {
		s.Log("/app (pid %d) exited with status %d after %s", pid, status, ran)
	}

	if ran >= s.StableAfter {
		s.Backoff.Reset()
	}
}
