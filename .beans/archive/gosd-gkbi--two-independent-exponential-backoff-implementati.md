---
# gosd-gkbi
title: 'Two independent exponential-backoff implementations: boot.Backoff never migrated onto childbackoff'
status: completed
type: task
priority: low
created_at: 2026-08-16T04:43:32Z
updated_at: 2026-08-20T05:39:32Z
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

- [x] Replace `boot.Backoff` usage in `cmd/gosd-init/internal/boot/supervisor.go`
      with `childbackoff.NewBackoff`
- [x] Delete `cmd/gosd-init/internal/boot/backoff.go`'s duplicate `Backoff`
      type (keep `StableRunThreshold` and the `Default*` constants)
- [x] Confirm `boot`'s existing backoff tests still pass, or fold them into
      `childbackoff`'s if they've become redundant

## Summary of Changes

Consolidated `/app`'s restart backoff onto `childbackoff.Backoff`, the same
engine `cloudflared` and `tsfunnel` already share:

- `boot/backoff.go`: deleted the duplicate `Backoff` type, `NewBackoff`, and
  its `Next`/`Reset` methods. Kept `DefaultBackoffBase` (1s), `DefaultBackoffCap`
  (10s) and `StableRunThreshold` (30s) — `/app`'s own restart-policy
  constants, unchanged.
- `boot/supervisor.go`: `Supervisor.Backoff` is now `*childbackoff.Backoff`.
- `boot/sequence.go`: constructs it via
  `childbackoff.NewBackoff(DefaultBackoffBase, DefaultBackoffCap)` —
  identical bounds, so identical timing.
- `boot/interfaces.go`: package doc updated to stop listing `Backoff` as
  living in this package.
- Test call sites (`supervisor_test.go`, `appfault_test.go`) updated to
  construct `childbackoff.NewBackoff` directly; behavior/assertions
  unchanged.
- `backoff_test.go` rewritten (not folded away, since `childbackoff`'s own
  tests use cloudflared's 1s/30s bounds, not boot's 1s/10s) to pin `/app`'s
  own `DefaultBackoffBase`/`DefaultBackoffCap` against the shared engine —
  a regression test for boot's specific bounds, not the doubling/capping
  algorithm itself.

No behavior change: bounds (1s base, 10s cap, 30s stable-run threshold) are
byte-for-byte what `boot.Backoff` already produced, and the full `boot`
package test suite (including the escalating-backoff and stable-reset
supervisor tests) passes unchanged.
