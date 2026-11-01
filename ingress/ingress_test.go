package ingress

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newLookup(t *testing.T) (*lookup, string) {
	t.Helper()
	dir := t.TempDir()
	return &lookup{dir: dir, pollInterval: time.Millisecond}, dir
}

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func timeout(t *testing.T, d time.Duration) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), d)
	t.Cleanup(cancel)
	return ctx
}

func TestURLReturnsOneWrittenBeforeTheCall(t *testing.T) {
	l, dir := newLookup(t)
	write(t, dir, "tailscale-funnel", "https://pi.tail1234.ts.net\n")

	u, err := l.url(timeout(t, time.Second))
	if err != nil || u.String() != "https://pi.tail1234.ts.net" {
		t.Fatalf("url() = %v, %v", u, err)
	}
}

func TestURLWaitsForOneWrittenAfterTheCall(t *testing.T) {
	l, dir := newLookup(t)
	go func() {
		time.Sleep(20 * time.Millisecond)
		_ = os.WriteFile(filepath.Join(dir, "cloudflared"), []byte("https://app.example.com"), 0o644)
	}()

	u, err := l.url(timeout(t, 5*time.Second))
	if err != nil || u.Host != "app.example.com" {
		t.Fatalf("url() = %v, %v", u, err)
	}
}

func TestURLPrefersCloudflared(t *testing.T) {
	l, dir := newLookup(t)
	write(t, dir, "tailscale-funnel", "https://pi.tail1234.ts.net")
	write(t, dir, "cloudflared", "https://app.example.com")

	u, err := l.url(timeout(t, time.Second))
	if err != nil || u.Host != "app.example.com" {
		t.Fatalf("url() = %v, %v", u, err)
	}
}

func TestURLTreatsEmptyOrGarbageAsNotYet(t *testing.T) {
	l, dir := newLookup(t)
	write(t, dir, "cloudflared", "")
	write(t, dir, "tailscale-funnel", "not a url")

	_, err := l.url(timeout(t, 30*time.Millisecond))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("url() error = %v, want context.DeadlineExceeded", err)
	}
}

func TestURLOffADeviceFailsImmediately(t *testing.T) {
	_, err := URL(timeout(t, time.Second))
	if err == nil || !strings.Contains(err.Error(), "gosd build") {
		t.Fatalf("URL() error = %v, want an actionable off-device error", err)
	}
}
