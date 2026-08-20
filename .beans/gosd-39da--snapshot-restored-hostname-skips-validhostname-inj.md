---
# gosd-39da
title: Snapshot-restored hostname skips validHostname, injecting arbitrary lines into /etc/hosts
status: completed
type: bug
priority: normal
created_at: 2026-08-12T04:13:59Z
updated_at: 2026-08-20T07:19:35Z
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

- [x] Gate the snapshot-restore hostname on `validHostname`, with a matching log line
- [x] Reject control characters in `hostsfile.Render` regardless of caller
- [x] Test: a snapshot hostname containing `\n` cannot add a line to /etc/hosts
- [x] Audit the other snapshot-restored fields (WiFi SSID, env keys/values, ingress) for the same missing-gate pattern

## Summary of Changes

Shipped with `gosd-7m9y` in one PR: the two are the same defect seen from
each end — an unauthenticated `/data` on one side, an ungated sink on the
other.

**The gate this bean asks for already exists, and did not when the bean was
written.** Epic `gosd-rw6n` replaced `provsnapshot`'s `gosd.toml` with the
per-file config tree and `cmd/gosd-init/internal/configstore` in between,
and in doing so restructured the restore path: a restored value is now
written onto the card and read back out of the tree, so
`sequence.go`'s restore branch re-resolves the hostname through
`cardHostname` → `validHostname` rather than assigning it raw. The line
`cfg.Hostname = gosdToml.Hostname` no longer exists anywhere. **That is a
structural fix nobody made deliberately**, which is exactly the kind that
gets refactored back out, so it is now pinned by a test rather than left as
a property of the current shape.

What actually changed:

- **`internal/hostsfile`: the sink no longer trusts its caller.** `Render`
  emits no `127.0.1.1` line at all for a hostname `naming.ValidHostname`
  refuses, keeping the static localhost lines either way. This is the
  component whose output format the injection targets, so it is the one
  that must not depend on being called correctly.
- **One definition of "valid hostname", in `internal/naming`.**
  `naming.ValidHostname` is new; `boot.validHostname` now delegates to it
  and `hostsfile.Render` calls it. Two copies of this rule is how a
  hostname carrying a newline reached `/etc/hosts` in the first place.
- **Regression test, end to end and through the real renderer**
  (`TestRunRefusesARestoredHostnameThatWouldForgeAnEtcHostsLine`): a
  hostname of `evil\n1.2.3.4 api.vendor.example` is planted the way a real
  one enters the store, a different image identity boots, and the rendered
  `/etc/hosts` is asserted to contain no attacker-chosen mapping. Plus unit
  tests on `Render` itself, which is the layer that would catch a future
  caller that skips the gate.

## Audit of the other restored fields

Done against every setting the tree can carry, tracing each to the sink it
actually reaches. Two needed work, three did not:

| Setting | Sink | Verdict |
|---|---|---|
| `hostname` | `/etc/hosts` (line-oriented, unescaped) | **was the bug**; gated at the reader, now also at the sink |
| `env/<NAME>` names | `execve(2)` env | **gap found**: the build refuses a malformed name, the runtime did not. `mergeUserEnv` now applies `configtree.ValidEnvName`, the same rule, to card and store alike |
| any value | `execve(2)` env | **gap found**: a NUL makes `execve` fail with EINVAL, so one planted in the store would stop `/app` starting on every boot *and survive the reflash performed to fix it*. `configtree.PlausibleValue` now refuses one, in both `cardconfig` and the store. Deliberately no stricter — a multi-line value pasted into `config/env/` is legitimate |
| `wifi/ssid`, `wifi/passphrase` | nl80211 netlink attributes | no text-format sink; length-delimited binary attributes, no `wpa_supplicant.conf` is ever written. No gate needed |
| `ingress/cloudflared/hostname`, `port` | `config.yml` | already gated: `cloudflared/mode.go`'s own `validHostname` exists precisely to stop a break-out of the ingress rule's line |
| `ingress/tailscale-funnel/*` | shim argv | argv elements, not a shell string; no injection. A NUL would fail the exec, now refused upstream |
| `ingress/*/token`, `authkey` | tunnel authorisation | not a validation problem — see `gosd-7m9y`; never restored at all now |
