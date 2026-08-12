---
# gosd-66ax
title: Wire cloudflared into StartNetworking + amend the single-child supervision contract
status: completed
type: task
priority: normal
created_at: 2026-08-07T12:52:39Z
updated_at: 2026-08-07T18:22:24Z
parent: gosd-virc
blocked_by:
    - gosd-g4km
    - gosd-uj36
---

Ingress epic gosd-virc bean 4 (needs gosd-g4km's initcfg field + gosd-uj36's
module). Soft dependency: gosd-e3xi (/etc/hosts) merged before this boots on
hardware — the generated rule targets `localhost`.

## Locked decisions

- One `guard.Go("cloudflared", ...)` inside StartNetworking
  (cmd/gosd-init/main.go ~L112-171) — it already receives cfg + gosdToml, so
  NO boot.Deps signature change. Deps wired: StartProcess → package starter,
  Wait → platform.Reaper.Wait, markers → netup.DefaultNetworkUpPath /
  timesync.DefaultTimeSyncedPath stat closures, files → os funcs.
- Same-PR contract amendments (the gosd-oyhi carve-out — gosd-shipped system
  services may be gosd-init-supervised; user externals stay app-owned):
  [x] boot/reaper.go L63-66 stash comment ("single child at a time" → "small,
      fixed set of children ... Wait called as soon as each Start returns";
      eviction argument survives, window logic is per-pid)
  [x] docs/runtime.md ~L825 single-child bullet reworded
  [x] gosd-oyhi bean body gains the carve-out record
  [x] CLAUDE.md's "gosd-init has no interactive surface" locked-decision
      bullet gains one clarifying clause: cloudflared (when baked via
      --ingress) is an outbound-only tunnel supervised by gosd-init — still
      no listeners, no shell.
- reaper: add a test proving two concurrent Waiters on distinct pids both
  resolve (pins the amended comment's claim).
- qemu smoke (CI): `gosd run --ingress cloudflared` with no section → exactly
  the one "baked but not configured" line; with a dummy token → supervisor
  start/exit/backoff lines (auth failure IS the assertion). Real tunnel
  establishment is bench-only (final epic bean).

  Implemented: the "no section" case, as its own additive step in the
  existing `qemu-boot` CI job. The dummy-token supervisor-loop case is
  SKIPPED: reaching that far means cloudflared itself dials the real
  Cloudflare edge and logs its own auth-failure wording, which is (a) a
  genuine network expectation for a CI runner beyond what every other job
  in this file needs, and (b) an upstream cloudflared log-format detail
  this repo doesn't control and shouldn't pin a CI assertion to. Deferred to
  bench verification, per the locked decision's own "real tunnel
  establishment is bench-only" clause.

## Summary of Changes

- `cmd/gosd-init/main.go`: added `guard.Go("cloudflared", ...)` inside
  `StartNetworking`, placed before the WiFi-hardware-absent early return so
  an Ethernet-only board never skips it, plus the `cloudflaredDeps`
  constructor (StartProcess → `cloudflared.StartProcess`, Wait →
  `platform.Reaper.Wait`, NetworkUp → the existing
  `timesync.NetworkUpMarkerExists(netup.DefaultNetworkUpPath)` closure
  reused verbatim, TimeSynced → the new `timeSyncedMarkerExists` local
  helper, files → `os.MkdirAll`/`os.WriteFile`, Clock →
  `cloudflared.NewRealClock()`) and the `cloudflaredBinaryPath` constant
  (`/bin/cloudflared`) near `appPath`.
- `cmd/gosd-init/internal/cloudflared/clock.go` (new): `NewRealClock()`,
  mirroring netup's/timesync's `realClock` exactly — the module shipped
  unwired with no production Clock implementation, since only its wiring
  bean needed one.
- `cmd/gosd-init/internal/boot/reaper.go`: reworded the stash comment's
  "single child at a time" eviction argument to "small, fixed set of
  children ... Wait called as soon as each Start returns" — the argument
  still holds, per-pid, now that cloudflared can run alongside `/app`.
- `cmd/gosd-init/internal/boot/reaper_test.go`: added
  `TestConcurrentWaitersOnDistinctPidsBothResolve`, which parks two `Wait`
  goroutines on distinct pids before either is reaped, pinning the amended
  comment's claim.
- `docs/runtime.md`: reworded the `--with-external` section's "Your app owns
  it at runtime" bullet to name the gosd-shipped-service carve-out
  (cloudflared) alongside the still-unchanged "user externals are app-owned"
  rule.
- `.beans/gosd-oyhi--*.md`: converted the "Amendment planned" note into a
  recorded amendment, describing the actual wiring (guard.Go, PID-1 reaper)
  landed here.
- `CLAUDE.md`: the "gosd-init has no interactive surface" locked decision
  gains one sentence naming cloudflared as an outbound-only, gosd-init
  supervised exception that adds no listener and no shell.
- `.github/workflows/ci.yml`: one additive step in the `qemu-boot` job
  building the gosd CLI and running `gosd run --ingress cloudflared` against
  `examples/hello` with no `[ingress.cloudflared]` section, asserting the
  "baked but not configured" line appears exactly once. Minimal/additive per
  the task's instruction; a trivial rebase conflict is expected once this
  stack lands on `main`, since `ci.yml` there has since picked up SHA-pinning
  (gosd-vag9, PR #209) and an HCTOSYS grep (gosd-jyq8, PR #211) that this
  branch's `ci.yml` predates.
- `cmd/gosd-init/internal/cloudflared/{cloudflared,mode,platform}.go`:
  unchanged — this bean only wires the pre-existing, already-reviewed
  module (gosd-uj36) into the boot sequence.
