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
6. **Deliberate limits** (amended 2026-11-01, JP): the file is an address, not a liveness signal, and isn't removed when the tunnel drops. This REVERSES the original "no 'this device has no tunnel' signal": wherever gosd-init already knows no tunnel will come up this boot, it writes an "off" record carrying the actionable line it logs, and `ingress.URL` fails fast with `ErrNoTunnel` once every agent is off. Final cases only (nothing baked, card settings empty or invalid, local setup failing); a tunnel that is retrying or still coming up (bad auth key, ACL, slow first registration) is never marked off, so callers still pass a deadline.
7. **Record format** (2026-11-01): each file is a JSON `{"url"}` or `{"off"}` rather than a bare URL, so a torn read is invalid and still means "not yet" (the `wifictl`/`faultdrop` precedent). A missing, empty or invalid file is "not yet" for that agent; `Read` reports "off" only when every agent's record says so.

## Todo

- [x] `internal/ingressurl` contract package
- [x] cloudflared supervisor writes its URL
- [x] gosd-tsfunnel `--url-file` + `publicURL`; gosd-init passes the flag
- [x] public `ingress` package with behavioural tests
- [x] docs/ingress.md section: `r.Host` in handlers, `ingress.URL` outside them
- [x] fail fast: JSON records, "off" written where gosd-init's decision is final, `ingress.ErrNoTunnel`
- [x] change file

## Summary of Changes

- `internal/ingressurl`: shared contract (`Dir`, `CloudflaredPath`, `TailscaleFunnelPath`, JSON `Record` with `URLRecord`/`OffRecord` encoders, `Read` preferring cloudflared; missing/empty/invalid is "not yet", and every agent off yields their reasons).
- cloudflared supervisor writes `https://<hostname>` right after its runtime files; a write failure is logged, not fatal.
- `gosd-tsfunnel` takes an optional `--url-file` and writes `publicURL(Self.DNSName, port)` once `ListenFunnel` succeeds; gosd-init passes the flag.
- cloudflared and tailscale-funnel supervisors write an "off" record with their logged reason when mode resolution says not to run or local setup (runtime files, state dir) fails; `tsfunnel.Deps` gains `WriteFile`. Not written on shutdown or when a child crashes or loops.
- New public `ingress` package: `URL(ctx)` (returns `ErrNoTunnel` immediately when every agent is off), `gosd` build-tag gate like `wifi`, immediate error off a device.
- docs/ingress.md gains "Your app's own public URL" covering `ErrNoTunnel`; a minor change file was added.
