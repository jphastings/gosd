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
// [URL] fails fast, with [ErrNoTunnel], wherever gosd-init already knows no
// tunnel will come up this boot: no `--ingress` baked into the image, the
// card's settings empty or invalid, or a tunnel's local setup failing. It
// still waits while a tunnel is retrying or still coming up (Funnel's first
// registration can take minutes, and a bad auth key or tailnet ACL is fixed
// outside the device), so always pass a deadline. The file is an address, not
// a liveness signal — it is not removed when the tunnel drops, only
// rewritten when the tunnel next comes up.
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
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/jphastings/gosd/internal/ingressurl"
)

// ErrNoTunnel is returned (wrapped, with the reasons) by [URL] when
// gosd-init has decided no ingress tunnel will come up on this device this
// boot.
var ErrNoTunnel = errors.New("ingress: no tunnel will come up on this device")

// defaultPollInterval is how often URL checks for a recorded URL.
const defaultPollInterval = 500 * time.Millisecond

// URL returns the public URL this device's ingress tunnel serves the app at,
// waiting until a tunnel reports one or ctx is done, in which case it
// returns ctx.Err(). If gosd-init reports that no tunnel will come up this
// boot, it returns an error wrapping [ErrNoTunnel] straight away, naming
// why.
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
		u, off := ingressurl.Read(l.dir)
		if u != nil {
			return u, nil
		}
		if len(off) > 0 {
			return nil, fmt.Errorf("%w: %s", ErrNoTunnel, strings.Join(off, "; "))
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}
