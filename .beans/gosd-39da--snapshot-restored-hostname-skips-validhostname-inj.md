---
# gosd-39da
title: Snapshot-restored hostname skips validHostname, injecting arbitrary lines into /etc/hosts
status: todo
type: bug
created_at: 2026-08-12T04:13:59Z
updated_at: 2026-08-12T04:13:59Z
---

**Severity: High.** A one-line inconsistency in a single function, reachable
from unauthenticated `/data` content, that yields DNS spoofing for the app.

## Verified — three set sites, one missing its gate

In `cmd/gosd-init/internal/boot/sequence.go`, `cfg.Hostname` is set from card
content in three places:

- **cloud-init**, `:310-317` — `if validHostname(provisionResult.Hostname) { ... }` — gated
- **gosd.toml**, `:327-334` — `if validHostname(gosdToml.Hostname) { ... }` — gated
- **snapshot restore**, `:405-407` — `cfg.Hostname = gosdToml.Hostname` — **no gate**

`validHostname` (`:856-858`) is `name != "" && naming.Sanitize(name) == name`,
restricting to `[a-z0-9-]` with a 63-byte cap. The snapshot path calls
`SetHostname` and then, at `:423-427`, `deps.WriteHosts(cfg.Hostname)`.

`internal/hostsfile/hostsfile.go:67-69` renders with
`fmt.Sprintf("127.0.1.1 %s\n", hostname)` — no escaping.

TOML basic strings encode arbitrary bytes via `\uXXXX`, so a newline or NUL
in this value is legal and survives `gosdtoml.Parse` unchanged (confirmed
separately: an `[env]` key with `\n` round-trips intact and raises no
warning).

## Attack

Given a planted `/data` snapshot (see the snapshot-authenticity bean — the
digest is unkeyed, so the attacker computes a valid one), set
`Effective.Hostname` to:

```
evil\n1.2.3.4 api.vendor.example
```

On the next reflash — which the owner performs believing it resets the
device — `heal()` fires (identity differs), the value is restored with no
validation, and `/etc/hosts` gains an attacker-chosen line. Every gosd image
is `CGO_ENABLED=0`, so Go's pure resolver consults `/etc/hosts` first for
every lookup the app makes: the app's API or update endpoint now resolves to
the attacker's address, on a device the owner just "factory reset".

## Fix

Apply the gate that already exists three lines above, at
sequence.go:405:

```go
if snapshot.HostnameRestored && validHostname(gosdToml.Hostname) {
```

with an `else` log line matching the other two sites' wording. No new
validation to design.

Harden the sink too, independently: `hostsfile.Render` should reject or
strip any hostname containing a control character rather than trusting its
callers, since it is the component whose output format the injection
targets.

## Todos

- [ ] Gate the snapshot-restore hostname on `validHostname`, with a matching log line
- [ ] Reject control characters in `hostsfile.Render` regardless of caller
- [ ] Test: a snapshot hostname containing `\n` cannot add a line to /etc/hosts
- [ ] Audit the other snapshot-restored fields (WiFi SSID, env keys/values, ingress) for the same missing-gate pattern
