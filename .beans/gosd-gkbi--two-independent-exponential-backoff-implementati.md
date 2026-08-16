---
# gosd-gkbi
title: 'Two independent exponential-backoff implementations: boot.Backoff never migrated onto childbackoff'
status: todo
type: task
priority: low
created_at: 2026-08-16T04:43:32Z
updated_at: 2026-08-16T04:43:32Z
---

**Severity: Low.** Both implementations are correct and tested — this is
duplication, not a bug — but it's exactly the kind of drift the extraction
was meant to prevent, and it will confuse the next person who greps for
"the" backoff engine and finds two.

## Verified

`cmd/gosd-init/internal/childbackoff/backoff.go`'s package doc says it was
"extracted from `cmd/gosd-init/internal/cloudflared/backoff.go` (bean
`gosd-wxjy`) so a second gosd-init-supervised agent (epic `gosd-65uy`) can
reuse the exact same doubling/capping engine instead of duplicating it."
`cloudflared` and `tsfunnel` both use it today (`main.go:618` wires
`childbackoff.NewBackoff` into `cloudflaredDeps`; `tsfunnel`'s equivalent
matches).

`cmd/gosd-init/internal/boot/backoff.go` — the `/app` supervisor's own
backoff, used by `Supervisor` for restarting the app itself — was never
migrated. Its `Backoff` type is structurally identical to `childbackoff`'s:
same fields (`base, max, delay time.Duration`), same `NewBackoff`,
`Next` (double-and-cap), and `Reset` bodies, byte-for-byte. The only
non-duplicated part is `boot.StableRunThreshold`, which is `/app`'s own
restart policy (correctly, per `childbackoff`'s doc, callers own their
stability threshold) — not a reason the doubling/capping engine itself
needs its own copy.

## Fix

Point `boot.Backoff`'s call sites at `childbackoff.NewBackoff`
(`DefaultBackoffBase`/`DefaultBackoffCap` become plain `time.Duration`
constants passed in, same as `cloudflared` already does) and delete
`boot/backoff.go`'s duplicate type, keeping `StableRunThreshold` where it is
since it's policy, not engine. Small, low-risk cleanup — `boot/backoff_test.go`
should still pass unchanged against the shared implementation's behavior.

## Todos

- [ ] Replace `boot.Backoff` usage in `cmd/gosd-init/internal/boot/supervisor.go`
      with `childbackoff.NewBackoff`
- [ ] Delete `cmd/gosd-init/internal/boot/backoff.go`'s duplicate `Backoff`
      type (keep `StableRunThreshold` and the `Default*` constants)
- [ ] Confirm `boot`'s existing backoff tests still pass, or fold them into
      `childbackoff`'s if they've become redundant
