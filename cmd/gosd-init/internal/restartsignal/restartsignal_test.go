package restartsignal

import (
	"errors"
	"testing"
)

func TestSignalCoalescesBurstsIntoOnePendingNotification(t *testing.T) {
	s := NewSignal()
	s.Notify()
	s.Notify()
	s.Notify()

	select {
	case <-s.C():
	default:
		t.Fatal("expected a pending notification after Notify")
	}

	select {
	case <-s.C():
		t.Fatal("expected exactly one coalesced notification, got a second")
	default:
	}
}

func TestSignalNotifyBeforeAnyReceiverIsNotLost(t *testing.T) {
	s := NewSignal()
	s.Notify() // nobody is reading s.C() yet

	select {
	case <-s.C():
	default:
		t.Fatal("a notification sent before any receiver should still be delivered")
	}
}

func TestDrainDiscardsAPendingNotification(t *testing.T) {
	s := NewSignal()
	s.Notify()
	s.Drain()

	select {
	case <-s.C():
		t.Fatal("Drain should have discarded the pending notification")
	default:
	}
}

func TestDrainOnNilSignalIsANoop(t *testing.T) {
	var s *Signal
	s.Drain() // must not panic
}

func TestWaitOrKillReturnsWaitResultWhenNilSignal(t *testing.T) {
	wantErr := errors.New("boom")
	status, err, restarted := WaitOrKill(nil, func() (int, error) { return 42, wantErr }, func() {
		t.Fatal("kill must never be called for a nil signal")
	})

	if status != 42 || !errors.Is(err, wantErr) || restarted {
		t.Fatalf("WaitOrKill = (%d, %v, %v), want (42, %v, false)", status, err, restarted, wantErr)
	}
}

func TestWaitOrKillReturnsWaitResultWhenSignalNeverFires(t *testing.T) {
	s := NewSignal()
	status, err, restarted := WaitOrKill(s, func() (int, error) { return 7, nil }, func() {
		t.Fatal("kill must never be called when the signal never fires")
	})

	if status != 7 || err != nil || restarted {
		t.Fatalf("WaitOrKill = (%d, %v, %v), want (7, nil, false)", status, err, restarted)
	}
}

func TestWaitOrKillCallsKillAndWaitsForRealExitWhenSignalFires(t *testing.T) {
	s := NewSignal()
	s.Notify()

	waitDone := make(chan struct{})
	killed := false
	status, err, restarted := WaitOrKill(s, func() (int, error) {
		<-waitDone // simulates the child only actually exiting once killed
		return 9, nil
	}, func() {
		killed = true
		close(waitDone)
	})

	if !killed {
		t.Fatal("kill was never called despite the signal firing")
	}
	if status != 9 || err != nil || !restarted {
		t.Fatalf("WaitOrKill = (%d, %v, %v), want (9, nil, true)", status, err, restarted)
	}
}
