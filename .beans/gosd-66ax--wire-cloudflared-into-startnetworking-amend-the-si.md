---
# gosd-66ax
title: Wire cloudflared into StartNetworking + amend the single-child supervision contract
status: todo
type: task
priority: normal
created_at: 2026-08-07T12:52:39Z
updated_at: 2026-08-07T12:53:22Z
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
  [ ] boot/reaper.go L63-66 stash comment ("single child at a time" → "small,
      fixed set of children ... Wait called as soon as each Start returns";
      eviction argument survives, window logic is per-pid)
  [ ] docs/runtime.md ~L825 single-child bullet reworded
  [ ] gosd-oyhi bean body gains the carve-out record
- reaper: add a test proving two concurrent Waiters on distinct pids both
  resolve (pins the amended comment's claim).
- qemu smoke (CI): `gosd run --ingress cloudflared` with no section → exactly
  the one "baked but not configured" line; with a dummy token → supervisor
  start/exit/backoff lines (auth failure IS the assertion). Real tunnel
  establishment is bench-only (final epic bean).
