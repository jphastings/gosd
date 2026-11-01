// Package ingress lets a GoSD app learn the public URL its ingress tunnel
// (`gosd build --ingress cloudflared` or `tailscale-funnel`) serves it at,
// outside an HTTP request: for logging at startup, drawing a QR code on a
// display, or registering a webhook or OAuth redirect.
//
// Inside a handler none of this is needed: both tunnels deliver the public
// name as r.Host (with X-Forwarded-Proto: https), so the request already
// says where it was addressed.
//
// The URL travels from the tunnel's supervisor to the app as a file under
// /run/gosd, the same idiom as wifi and ready (see internal/ingressurl);
// gosd-init gains no listener. If both tunnels are baked in, cloudflared's
// URL is preferred.
//
// # Limits
//
// There is no "this device has no tunnel" signal: on a device with none
// configured, [URL] waits until its context ends, so always pass a deadline.
// The file is an address, not a liveness signal — it is not removed when the
// tunnel drops, only rewritten when the tunnel next comes up.
//
// # Off a device
//
// Like wifi, behaviour follows the `gosd` build tag rather than probing for
// /run, so a `go test` on a Linux CI runner never mistakes itself for a
// device. Off a device, URL returns an immediate, actionable error so an app
// can fall back to localhost in development.
package ingress

import (
	"context"
	"errors"
	"net/url"
	"time"

	"github.com/jphastings/gosd/internal/ingressurl"
)

// defaultPollInterval is how often URL checks for a recorded URL.
const defaultPollInterval = 500 * time.Millisecond

// URL returns the public URL this device's ingress tunnel serves the app at,
// waiting until a tunnel reports one or ctx is done, in which case it
// returns ctx.Err().
//
// Off a device — anything not built by `gosd build` — it returns an
// immediate, actionable error: see the package doc.
func URL(ctx context.Context) (*url.URL, error) {
	return std.url(ctx)
}

// std is the lookup behind [URL]; tests build their own against a temp dir.
var std = &lookup{dir: runDir}

type lookup struct {
	dir          string
	pollInterval time.Duration
}

func (l *lookup) url(ctx context.Context) (*url.URL, error) {
	if l.dir == "" {
		return nil, errors.New("ingress.URL only works in an app built by `gosd build`; this binary wasn't, so no tunnel exists to ask. Fall back to localhost (or r.Host inside a handler) when running locally")
	}
	interval := l.pollInterval
	if interval <= 0 {
		interval = defaultPollInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		if u, ok := ingressurl.Read(l.dir); ok {
			return u, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}
