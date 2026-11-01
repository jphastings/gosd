---
gosd: minor
---

#### Apps can learn their public ingress URL

A new `ingress` package lets an app find the public URL its Cloudflare Tunnel or Tailscale Funnel serves it at, outside an HTTP request: `ingress.URL(ctx)` waits until the tunnel reports its address. Inside a handler, `r.Host` already carries it. Pass a deadline, since a device with no tunnel never reports one.
