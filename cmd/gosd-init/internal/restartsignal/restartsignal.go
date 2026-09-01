// Package restartsignal implements a coalescing "restart your child now"
// notifier, shared by the gosd-init modules that supervise a single
// externally-launched child process (cloudflared, tsfunnel) and need to be
// told to restart it from elsewhere in gosd-init — the runtime-WiFi-join
// epic's ingress reconnect (gosd-ojbm decision 4) is the first producer.
// mdnsresponder.Signal is this package's in-process precedent and predates
// it; mdnsresponder keeps its own copy since it isn't a consumer of this
// feature and restarts an in-process responder rather than an external
// child. cloudflared and tsfunnel are near-twins that already share
// childbackoff and logwriter (bean gosd-wxjy) for exactly this reason: a
// third identical copy of the same plumbing is worse than one shared
// package.
package restartsignal

// Signal is a coalescing change notifier: any number of Notify calls before
// the receiver next reads from C collapse into a single pending wakeup, so a
// burst of restart requests triggers one restart, not one per request. The
// zero value is not usable; construct with NewSignal.
type Signal struct {
	ch chan struct{}
}

// NewSignal returns a ready-to-use Signal.
func NewSignal() *Signal {
	return &Signal{ch: make(chan struct{}, 1)}
}

// Notify records that a restart was requested. Non-blocking: if a
// notification is already pending, this is a no-op.
func (s *Signal) Notify() {
	select {
	case s.ch <- struct{}{}:
	default:
	}
}

// C returns the channel to select on. Buffered (capacity 1), so a Notify
// call that happens before anything is listening on C isn't lost.
func (s *Signal) C() <-chan struct{} {
	return s.ch
}

// Drain discards any pending notification without acting on it, so a Notify
// that arrived while nobody was ready to respond (e.g. while a supervisor is
// parked waiting for the network to come up, or during the backoff sleep
// between children) coalesces harmlessly instead of triggering an extra
// restart once a receiver is ready again. Safe to call on a nil *Signal (a
// no-op), so callers holding a possibly-unset Deps field don't need to
// nil-check first.
func (s *Signal) Drain() {
	if s == nil {
		return
	}
	select {
	case <-s.ch:
	default:
	}
}

// WaitOrKill blocks on wait (typically a supervisor's Deps.Wait(pid) call)
// until it returns, unless sig fires first — in which case it calls kill
// (typically Deps.Kill(pid), asking the child to terminate) and then still
// blocks on wait for the real exit, since sending a termination signal only
// asks a process to end; something must still reap its exit status.
// restarted reports whether kill fired, so the caller can treat this as a
// deliberate restart rather than a crash (e.g. resetting a restart backoff —
// epic gosd-ojbm decision 4: a deliberate restart is not a crash-loop
// signal). A nil sig makes this exactly wait(), with kill never called: this
// is what makes a nil restart signal behave exactly as if this package
// didn't exist.
func WaitOrKill(sig *Signal, wait func() (int, error), kill func()) (status int, err error, restarted bool) {
	if sig == nil {
		status, err = wait()
		return status, err, false
	}

	type result struct {
		status int
		err    error
	}
	done := make(chan result, 1)
	go func() {
		s, e := wait()
		done <- result{s, e}
	}()

	select {
	case r := <-done:
		return r.status, r.err, false
	case <-sig.C():
		kill()
		r := <-done
		return r.status, r.err, true
	}
}
