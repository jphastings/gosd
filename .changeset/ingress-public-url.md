---
gosd: minor
---

#### Apps can learn their public ingress URL

A new `ingress` package lets an app find the public URL its Cloudflare Tunnel or Tailscale Funnel serves it at, outside an HTTP request: `ingress.URL(ctx)` waits until the tunnel reports its address. Inside a handler, `r.Host` already carries it. It returns `ingress.ErrNoTunnel` immediately when the device knows no tunnel will come up this boot; otherwise pass a deadline, since a tunnel that is still coming up or retrying keeps it waiting.
