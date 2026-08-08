package main

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRegisterTimeoutError(t *testing.T) {
	inner := errors.New("context deadline exceeded")
	err := registerTimeoutError(5*time.Minute, inner)

	if !errors.Is(err, inner) {
		t.Error("registerTimeoutError does not wrap the underlying error")
	}
	for _, want := range []string{"5m0s", "TS_AUTHKEY", "expire", "clock"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error message %q missing %q", err.Error(), want)
		}
	}
}

func TestFunnelUnavailableError(t *testing.T) {
	inner := errors.New("Funnel not available; HTTPS must be enabled")
	err := funnelUnavailableError(inner)

	if !errors.Is(err, inner) {
		t.Error("funnelUnavailableError does not wrap the underlying error")
	}
	for _, want := range []string{"funnel", "HTTPS", "MagicDNS", "docs/ingress.md"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error message %q missing %q", err.Error(), want)
		}
	}
}
