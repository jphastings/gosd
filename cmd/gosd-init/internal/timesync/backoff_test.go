package timesync

import (
	"testing"
	"time"
)

func noJitterBackoff(base, max time.Duration) *Backoff {
	b := NewBackoff(base, max)
	b.random = func() float64 { return 1 }
	return b
}

func TestBackoffDoublesUntilCap(t *testing.T) {
	b := noJitterBackoff(time.Second, 8*time.Second)

	want := []time.Duration{1, 2, 4, 8, 8, 8}
	for i, w := range want {
		if got := b.Next(); got != w*time.Second {
			t.Errorf("Next() call %d = %s, want %s", i+1, got, w*time.Second)
		}
	}
}

func TestBackoffResetReturnsToBase(t *testing.T) {
	b := noJitterBackoff(time.Second, 8*time.Second)
	b.Next()
	b.Next()
	b.Reset()

	if got := b.Next(); got != time.Second {
		t.Errorf("Next() after Reset() = %s, want %s (base)", got, time.Second)
	}
}

func TestBackoffJitterStaysWithinBounds(t *testing.T) {
	b := NewBackoff(time.Second, 4*time.Second)
	for i := 0; i < 20; i++ {
		d := b.Next()
		if d <= 0 {
			t.Fatalf("Next() = %s, want > 0 (a zero-length wait would busy-loop)", d)
		}
		if d > 4*time.Second {
			t.Fatalf("Next() = %s, want <= cap (%s)", d, 4*time.Second)
		}
	}
}
