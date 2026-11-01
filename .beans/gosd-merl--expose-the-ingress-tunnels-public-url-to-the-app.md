---
# gosd-merl
title: Expose the ingress tunnel's public URL to the app
status: completed
type: feature
created_at: 2026-11-01T09:22:52Z
updated_at: 2026-11-01T09:22:52Z
---

Let an app learn the public URL its ingress tunnel (cloudflared or tailscale-funnel) serves it at, outside an HTTP request — for startup logging, a QR code on a display, webhook/OAuth registration.

Inside a handler this already works with no code: both tunnels deliver the public name as `r.Host` (gosd-tsfunnel's proxy keeps the inbound Host deliberately; cloudflared's generated config sets no `httpHostHeader`). An env var can't carry it: Funnel's `<name>.<tailnet>.ts.net` only exists once tsnet's `Up` returns, and a collision can rename the node to `<hostname>-1`.

## Locked decisions

1. **File drop under `/run/gosd`**, the `ready`/`wifi` idiom: a shared contract package `internal/ingressurl` (writer and reader can't drift). One file per agent, named after its `--ingress` value, under `/run/gosd/ingress/` — `--ingress` is repeatable, so both tunnels can run at once.
2. **Read preference is cloudflared, then tailscale-funnel** (deterministic; a custom domain is the "real" address). A missing, empty or unparseable file is "not yet", so writers use plain `WriteFile`, no temp+rename.
3. **cloudflared**: gosd-init's supervisor writes `https://<hostname>` right after its runtime files, via the existing Deps seams.
4. **tailscale-funnel**: the shim takes an optional `--url-file`, and writes `https://<Self.DNSName minus trailing dot>[:port unless 443]` from `srv.Up`'s status once `ListenFunnel` succeeds. A failed write logs, never fails the tunnel. gosd-init passes the flag; the shim restarts on every WiFi rejoin, so a rename is picked up.
5. **Public API**: new `ingress/` package, `URL(ctx) (*url.URL, error)`, waits until a tunnel reports or ctx ends; `gosd` build-tag gate like `wifi`; off a device it errors immediately and actionably.
6. **Deliberate limits**: no "this device has no tunnel" signal (URL waits until ctx — callers pass a deadline); the file is an address, not a liveness signal, and isn't removed when the tunnel drops.

## Todo

- [x] `internal/ingressurl` contract package
- [x] cloudflared supervisor writes its URL
- [x] gosd-tsfunnel `--url-file` + `publicURL`; gosd-init passes the flag
- [x] public `ingress` package with behavioural tests
- [x] docs/ingress.md section: `r.Host` in handlers, `ingress.URL` outside them
- [x] change file

## Summary of Changes

- `internal/ingressurl`: shared contract (`Dir`, `CloudflaredPath`, `TailscaleFunnelPath`, `Read` preferring cloudflared; missing/empty/unparseable is "not yet").
- cloudflared supervisor writes `https://<hostname>` right after its runtime files; a write failure is logged, not fatal.
- `gosd-tsfunnel` takes an optional `--url-file` and writes `publicURL(Self.DNSName, port)` once `ListenFunnel` succeeds; gosd-init passes the flag.
- New public `ingress` package: `URL(ctx)`, `gosd` build-tag gate like `wifi`, immediate error off a device.
- docs/ingress.md gains "Your app's own public URL"; a minor change file was added.
