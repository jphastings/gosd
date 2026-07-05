package mdnsresponder

// Signal is a coalescing change notifier: any number of Notify calls before
// the receiver next reads from C collapse into a single pending wakeup, so
// a burst of network events (several link/lease changes in quick
// succession) triggers one responder restart, not one per event. The zero
// value is not usable; construct with NewSignal.
type Signal struct {
	ch chan struct{}
}

// NewSignal returns a ready-to-use Signal.
func NewSignal() *Signal {
	return &Signal{ch: make(chan struct{}, 1)}
}

// Notify records that something changed. Non-blocking: if a notification is
// already pending, this is a no-op.
func (s *Signal) Notify() {
	select {
	case s.ch <- struct{}{}:
	default:
	}
}

// C returns the channel Run selects on. Buffered (capacity 1), so a Notify
// call that happens before anything is listening on C isn't lost.
func (s *Signal) C() <-chan struct{} {
	return s.ch
}
