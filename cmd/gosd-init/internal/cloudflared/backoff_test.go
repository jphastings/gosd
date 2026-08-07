package cloudflared

import (
	"testing"
	"time"
)

func TestBackoffDoublesAndCaps(t *testing.T) {
	b := NewBackoff(1*time.Second, 30*time.Second)

	want := []time.Duration{
		1 * time.Second,
		2 * time.Second,
		4 * time.Second,
		8 * time.Second,
		16 * time.Second,
		30 * time.Second, // capped
		30 * time.Second, // stays capped
	}
	for i, w := range want {
		if got := b.Next(); got != w {
			t.Fatalf("Next() call %d = %s, want %s", i+1, got, w)
		}
	}
}

func TestBackoffReset(t *testing.T) {
	b := NewBackoff(1*time.Second, 30*time.Second)
	b.Next()
	b.Next()
	b.Reset()

	if got := b.Next(); got != 1*time.Second {
		t.Fatalf("Next() after Reset = %s, want 1s", got)
	}
}

func TestDefaultBackoffBoundsMatchLockedDecision(t *testing.T) {
	if DefaultBackoffBase != time.Second {
		t.Errorf("DefaultBackoffBase = %s, want 1s", DefaultBackoffBase)
	}
	if DefaultBackoffCap != 30*time.Second {
		t.Errorf("DefaultBackoffCap = %s, want 30s", DefaultBackoffCap)
	}
	if StableAfter != 30*time.Second {
		t.Errorf("StableAfter = %s, want 30s", StableAfter)
	}
}
