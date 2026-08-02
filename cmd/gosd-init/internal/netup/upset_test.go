package netup

import "testing"

func newTestUpSet() (*UpSet, *counter, *counter) {
	marked := &counter{}
	cleared := &counter{}
	s := NewUpSet(
		func() error { marked.inc(); return nil },
		func() error { cleared.inc(); return nil },
	)
	return s, marked, cleared
}

func TestUpSetMarksOnFirstInterfaceUp(t *testing.T) {
	s, marked, cleared := newTestUpSet()

	if err := s.Up("eth0"); err != nil {
		t.Fatalf("Up() = %v", err)
	}
	if marked.load() != 1 {
		t.Errorf("marked = %d, want 1", marked.load())
	}
	if cleared.load() != 0 {
		t.Errorf("cleared = %d, want 0", cleared.load())
	}
}

func TestUpSetSecondInterfaceUpDoesNotRemark(t *testing.T) {
	s, marked, _ := newTestUpSet()

	if err := s.Up("eth0"); err != nil {
		t.Fatalf("Up(eth0) = %v", err)
	}
	if err := s.Up("wlan0"); err != nil {
		t.Fatalf("Up(wlan0) = %v", err)
	}
	if marked.load() != 1 {
		t.Errorf("marked = %d, want 1 (mark only fires on the empty->non-empty transition)", marked.load())
	}
}

// TestUpSetDualInterfaceOneDownLeavesMarkerUp is the bean's core scenario:
// a pi-3b with Ethernet and WiFi both up, then the cable is unplugged.
// The marker must stay because wlan0 still holds it.
func TestUpSetDualInterfaceOneDownLeavesMarkerUp(t *testing.T) {
	s, marked, cleared := newTestUpSet()

	if err := s.Up("eth0"); err != nil {
		t.Fatalf("Up(eth0) = %v", err)
	}
	if err := s.Up("wlan0"); err != nil {
		t.Fatalf("Up(wlan0) = %v", err)
	}
	if err := s.Down("eth0"); err != nil {
		t.Fatalf("Down(eth0) = %v", err)
	}

	if marked.load() != 1 {
		t.Errorf("marked = %d, want 1", marked.load())
	}
	if cleared.load() != 0 {
		t.Errorf("cleared = %d, want 0 (wlan0 is still up)", cleared.load())
	}
}

// TestUpSetDualInterfaceBothDownClears completes the scenario above: once
// wlan0 also goes down, the marker must finally be removed.
func TestUpSetDualInterfaceBothDownClears(t *testing.T) {
	s, _, cleared := newTestUpSet()

	if err := s.Up("eth0"); err != nil {
		t.Fatalf("Up(eth0) = %v", err)
	}
	if err := s.Up("wlan0"); err != nil {
		t.Fatalf("Up(wlan0) = %v", err)
	}
	if err := s.Down("eth0"); err != nil {
		t.Fatalf("Down(eth0) = %v", err)
	}
	if cleared.load() != 0 {
		t.Fatalf("cleared = %d, want 0 before the second interface goes down", cleared.load())
	}

	if err := s.Down("wlan0"); err != nil {
		t.Fatalf("Down(wlan0) = %v", err)
	}
	if cleared.load() != 1 {
		t.Errorf("cleared = %d, want 1 (both interfaces are now down)", cleared.load())
	}
}

// TestUpSetSingleInterfaceBehaviorUnchanged pins the Ethernet-only (or
// WiFi-only) case: mark on up, clear on down, exactly as the old
// unrefcounted boolean marker behaved.
func TestUpSetSingleInterfaceBehaviorUnchanged(t *testing.T) {
	s, marked, cleared := newTestUpSet()

	if err := s.Up("eth0"); err != nil {
		t.Fatalf("Up() = %v", err)
	}
	if err := s.Down("eth0"); err != nil {
		t.Fatalf("Down() = %v", err)
	}
	if marked.load() != 1 || cleared.load() != 1 {
		t.Errorf("marked=%d cleared=%d, want 1,1", marked.load(), cleared.load())
	}
}

// TestUpSetMarkerRecreatedWhenInterfaceReturns covers the replug/
// reassociate case: after the set empties and clear fires, the next
// interface to come back up must mark again, not assume it's still up.
func TestUpSetMarkerRecreatedWhenInterfaceReturns(t *testing.T) {
	s, marked, cleared := newTestUpSet()

	if err := s.Up("eth0"); err != nil {
		t.Fatalf("Up() = %v", err)
	}
	if err := s.Down("eth0"); err != nil {
		t.Fatalf("Down() = %v", err)
	}
	if err := s.Up("eth0"); err != nil {
		t.Fatalf("second Up() = %v", err)
	}

	if marked.load() != 2 {
		t.Errorf("marked = %d, want 2 (marker recreated after the interface returned)", marked.load())
	}
	if cleared.load() != 1 {
		t.Errorf("cleared = %d, want 1", cleared.load())
	}
}

func TestUpSetRedundantDownIsNoop(t *testing.T) {
	s, _, cleared := newTestUpSet()

	if err := s.Down("eth0"); err != nil {
		t.Fatalf("Down() on a never-up interface = %v", err)
	}
	if cleared.load() != 0 {
		t.Errorf("cleared = %d, want 0 (eth0 was never up)", cleared.load())
	}
}
