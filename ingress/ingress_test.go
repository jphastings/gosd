package ingress

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jphastings/gosd/internal/ingressurl"
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
	write(t, dir, "tailscale-funnel", string(ingressurl.URLRecord("https://pi.tail1234.ts.net")))

	u, err := l.url(timeout(t, time.Second))
	if err != nil || u.String() != "https://pi.tail1234.ts.net" {
		t.Fatalf("url() = %v, %v", u, err)
	}
}

func TestURLWaitsForOneWrittenAfterTheCall(t *testing.T) {
	l, dir := newLookup(t)
	go func() {
		time.Sleep(20 * time.Millisecond)
		_ = os.WriteFile(filepath.Join(dir, "cloudflared"), ingressurl.URLRecord("https://app.example.com"), 0o644)
	}()

	u, err := l.url(timeout(t, 5*time.Second))
	if err != nil || u.Host != "app.example.com" {
		t.Fatalf("url() = %v, %v", u, err)
	}
}

func TestURLPrefersCloudflared(t *testing.T) {
	l, dir := newLookup(t)
	write(t, dir, "tailscale-funnel", string(ingressurl.URLRecord("https://pi.tail1234.ts.net")))
	write(t, dir, "cloudflared", string(ingressurl.URLRecord("https://app.example.com")))

	u, err := l.url(timeout(t, time.Second))
	if err != nil || u.Host != "app.example.com" {
		t.Fatalf("url() = %v, %v", u, err)
	}
}

func TestURLTreatsEmptyOrGarbageAsNotYet(t *testing.T) {
	l, dir := newLookup(t)
	write(t, dir, "cloudflared", "")
	write(t, dir, "tailscale-funnel", string(ingressurl.URLRecord("not a url")))

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

func off(reason string) string { return string(ingressurl.OffRecord(reason)) }

func TestURLFailsFastWhenEveryAgentIsOff(t *testing.T) {
	l, dir := newLookup(t)
	write(t, dir, "cloudflared", off("cloudflared: not baked"))
	write(t, dir, "tailscale-funnel", off("tailscale-funnel: no port"))

	start := time.Now()
	_, err := l.url(timeout(t, 5*time.Second))
	if !errors.Is(err, ErrNoTunnel) {
		t.Fatalf("url() error = %v, want ErrNoTunnel", err)
	}
	for _, reason := range []string{"cloudflared: not baked", "tailscale-funnel: no port"} {
		if !strings.Contains(err.Error(), reason) {
			t.Errorf("error %q lacks reason %q", err, reason)
		}
	}
	if time.Since(start) > time.Second {
		t.Errorf("took %v, want an immediate failure", time.Since(start))
	}
}

func TestURLIgnoresAnOffAgentWhenAnotherHasAURL(t *testing.T) {
	l, dir := newLookup(t)
	write(t, dir, "cloudflared", off("cloudflared: not baked"))
	write(t, dir, "tailscale-funnel", string(ingressurl.URLRecord("https://pi.tail1234.ts.net")))

	u, err := l.url(timeout(t, time.Second))
	if err != nil || u.Host != "pi.tail1234.ts.net" {
		t.Fatalf("url() = %v, %v", u, err)
	}
}

func TestURLKeepsWaitingWhileTheOtherAgentIsUndecided(t *testing.T) {
	l, dir := newLookup(t)
	write(t, dir, "cloudflared", off("cloudflared: not baked"))

	_, err := l.url(timeout(t, 30*time.Millisecond))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("url() error = %v, want context.DeadlineExceeded", err)
	}
}
