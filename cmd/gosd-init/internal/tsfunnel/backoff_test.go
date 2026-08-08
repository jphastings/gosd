package tsfunnel

import (
	"testing"
	"time"
)

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
