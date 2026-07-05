package mdnsresponder

import "testing"

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
