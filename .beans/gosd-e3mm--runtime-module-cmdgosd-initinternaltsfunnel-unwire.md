---
# gosd-e3mm
title: Runtime module cmd/gosd-init/internal/tsfunnel (unwired, fake-tested)
status: todo
type: task
priority: normal
created_at: 2026-08-07T15:09:10Z
updated_at: 2026-08-07T15:10:32Z
parent: gosd-65uy
blocked_by:
    - gosd-85bn
    - gosd-uj36
---

Tailscale epic gosd-65uy bean 4 (after TS-1 gosd-85bn; needs cloudflared's
gosd-uj36, which lands the SHARED logwriter/childbackoff packages). Unwired —
reviewable in isolation; wiring is TS-5 gosd-o68e.

## Locked decisions

- cmd/gosd-init/internal/tsfunnel, feature-module shape, NO build tags (exec
  works on macOS; mdnsresponder precedent). Same Deps seam as the
  cloudflared module: StartProcess(path, args, env, stdout, stderr)
  package-local seam (boot's AppStarter takes no argv); Wait = the PID-1
  reaper's Wait, NEVER exec.Cmd.Wait (races the central wait4(-1) loop);
  NetworkUp/TimeSynced marker polls with paths injected (no netup/timesync
  imports); file funcs; Clock; Log (every line "tailscale-funnel: ...").
- Run() resolve step (pure; each misconfig row = ONE actionable line +
  return — gosd.toml is only re-read at boot): unconfigured+baked → one
  quiet line; configured+not-baked → points at --ingress tailscale-funnel;
  missing port → names the key; funnel_port outside {443, 8443, 10000} →
  names the allowed set. Hostname defaults to cfg.Hostname (the device
  hostname → the public URL's first label).
- Then: network-up gate (2s poll, parks forever) → time-synced ≤2min
  bounded wait, proceed with warning (control-plane TLS + ACME retry inside
  the shim/backoff) → STATE-DIR PREFLIGHT: MkdirAll /data/.gosd/tailscale;
  failure/EROFS → one actionable line ("tailscale-funnel needs a data
  partition; rebuild with --data-size") + return → supervise loop via
  shared logwriter/childbackoff: re-gate network-up before each start; log
  pid+argv (env NEVER — TS_AUTHKEY lives there); Wait; backoff sleep.
- argv per TS-2 gosd-4fve's flag contract; env TS_AUTHKEY=<authkey> (empty
  is fine once state exists — tsnet ignores it).
- Restart policy identical to cloudflared's: base 1s, cap 30s, reset after
  30s stable, NO jitter, NO restart on network change (pre-start re-gate
  parks the loop instead of burning backoff).
- Tests: fake-driven; argv/env exact-match; gate behavior; backoff sequence;
  every misconfig row's single line; state-dir preflight EROFS path;
  authkey-never-logged scan across ALL captured output.

## Todos

[ ] mode resolution + tests (misconfig rows, hostname default, funnel_port set)
[ ] state-dir preflight + tests (EROFS path)
[ ] Run loop + fakes (gates, backoff, stable reset, Stop)
[ ] authkey-never-logged scan test
