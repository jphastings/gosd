package boot

import (
	"testing"
	"time"

	"github.com/jphastings/gosd/cmd/gosd-init/internal/childbackoff"
)

// These pin /app's own restart-backoff bounds (1s base, 10s cap) against
// the shared childbackoff engine boot.Supervisor actually uses in
// production (see sequence.go) — a regression test for DefaultBackoffBase/
// DefaultBackoffCap themselves, not for the doubling/capping algorithm,
// which childbackoff's own tests already cover.

func TestSupervisorBackoffDoublesAndCaps(t *testing.T) {
	b := childbackoff.NewBackoff(DefaultBackoffBase, DefaultBackoffCap)

	want := []time.Duration{
		1 * time.Second,
		2 * time.Second,
		4 * time.Second,
		8 * time.Second,
		10 * time.Second, // capped
		10 * time.Second, // stays capped
	}
	for i, w := range want {
		if got := b.Next(); got != w {
			t.Fatalf("Next() call %d = %s, want %s", i+1, got, w)
		}
	}
}

func TestSupervisorBackoffReset(t *testing.T) {
	b := childbackoff.NewBackoff(DefaultBackoffBase, DefaultBackoffCap)
	b.Next()
	b.Next()
	b.Reset()

	if got := b.Next(); got != 1*time.Second {
		t.Fatalf("Next() after Reset = %s, want 1s", got)
	}
}
