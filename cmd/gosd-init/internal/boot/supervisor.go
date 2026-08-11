package boot

import "time"

// Supervisor runs /app, restarting it with exponential backoff whenever it
// exits, for as long as PID 1 lives. Every dependency is injected so the
// restart/backoff decisions can be unit-tested without real processes,
// clocks, or sleeps.
type Supervisor struct {
	// Start launches /app and returns its pid.
	Start func() (pid int, err error)
	// Wait blocks until pid has exited, returning everything the reaper
	// knows about how it died — not just an exit code, since a signal
	// death needs its own human-readable narration in a crash report (see
	// ExitStatus and OnExit below).
	Wait func(pid int) (ExitStatus, error)
	// Sleep pauses for the given duration between restart attempts.
	Sleep func(time.Duration)
	// Now returns the current time, used to measure how long /app ran.
	Now func() time.Time
	// After is the stable-run timer (see OnStableRun), injected so tests
	// don't wait out StableAfter in real time. Defaults to time.After.
	After func(time.Duration) <-chan time.Time
	// Backoff computes the delay before each restart attempt.
	Backoff *Backoff
	// StableAfter is how long /app must run before its next exit resets
	// Backoff back to its base delay.
	StableAfter time.Duration
	// OnStableRun, if non-nil, is called once /app has been running
	// continuously for StableAfter — from a timer, while it is still up,
	// not when it eventually exits. That distinction is the whole point:
	// the healthy case is an app that never exits at all, and it is
	// exactly the device that must stop looking broken (see
	// fatalReporter.markStableRun). It runs on its own goroutine, so it
	// must be safe to call concurrently with the app exiting.
	OnStableRun func()
	// OnExit, if non-nil, is called after every /app exit that Wait
	// reported without an error, with how it died and how long it ran.
	// Supervisor itself stays policy-free about what that means — it
	// always restarts /app regardless — so sequence.go is what decides
	// whether a given exit counts as a crash worth recording (gosd-s9uq).
	OnExit func(status ExitStatus, ran time.Duration)
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

	exited := s.watchForStableRun()
	status, err := s.Wait(pid)
	close(exited)
	ran := s.Now().Sub(startedAt)
	if err != nil {
		s.Log("supervising /app (pid %d) failed: %v", pid, err)
	} else {
		s.Log("/app (pid %d) exited with status %d after %s", pid, status.ExitCode, ran)
		if s.OnExit != nil {
			s.OnExit(status, ran)
		}
	}

	if ran >= s.StableAfter {
		s.Backoff.Reset()
	}
}

// watchForStableRun starts the timer behind OnStableRun and returns the
// channel the caller closes when /app exits, whichever happens first.
func (s *Supervisor) watchForStableRun() chan struct{} {
	exited := make(chan struct{})
	if s.OnStableRun == nil {
		return exited
	}

	after := s.After
	if after == nil {
		after = time.After
	}
	go func() {
		select {
		case <-exited:
		case <-after(s.StableAfter):
			s.OnStableRun()
		}
	}()
	return exited
}
