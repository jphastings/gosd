---
# gosd-e3mm
title: Runtime module cmd/gosd-init/internal/tsfunnel (unwired, fake-tested)
status: completed
type: task
priority: normal
created_at: 2026-08-07T15:09:10Z
updated_at: 2026-08-08T05:36:16Z
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

[x] mode resolution + tests (misconfig rows, hostname default, funnel_port set)
[x] state-dir preflight + tests (EROFS path)
[x] Run loop + fakes (gates, backoff, stable reset, Stop)
[x] authkey-never-logged scan test

## Summary of Changes

Implemented `cmd/gosd-init/internal/tsfunnel/` exactly per the locked
decisions, unwired (no changes to main.go/boot/any other package):

- `tsfunnel.go`: `Deps`/`Options`/`Run`. `Run` resolves the mode (pure),
  gates on network-up (2s poll, parks forever), gates on time-synced (up to
  2 minutes, then proceeds with a warning), preflights StateDir
  (`/data/.gosd/tailscale`, epic decision 3), then supervises the shim:
  re-gating network-up before every single start (parking, not backing off,
  if the network is down), logging pid+argv only (never env — TS_AUTHKEY
  lives there), and resetting backoff after a StableAfter-length run.
  `Deps.Wait`'s doc comment documents that production must wire
  `boot.Platform.Reaper.Wait`, never `exec.Cmd.Wait`, which would race the
  reaper's central wait4(-1) loop — mirrors cloudflared's Deps.Wait exactly.
  Unlike cloudflared (whose argv/env are fully static package vars, since
  its dynamic values are written into a config.yml file instead),
  tsfunnel's argv genuinely depends on the resolved hostname/port/
  funnel_port, so `runArgs`/`runEnv` take the resolved mode and
  `supervise`/`runOnce` carry it through explicitly.
- `mode.go`: `resolveMode` (pure — no I/O) covers every locked failure row
  (unconfigured/baked cross-product, missing port, port range 1-65535,
  funnel_port outside {443, 8443, 10000}) each producing at most one
  actionable log line, plus hostname defaulting to the caller-supplied
  device hostname when `[ingress.tailscale-funnel]` doesn't set one.
  Unlike cloudflared's token/hostname/port trio, only `port` is a required
  key: the auth key is needed solely for first tailnet registration (epic
  decision 4) and is never validated for shape, and hostname always has a
  default so it can never be "missing".
- `interfaces.go`/`clock.go`: `Clock` (Now/After) and its real
  implementation, duplicated per precedent (cloudflared/netup/timesync all
  keep their own copy rather than sharing one).
- `platform.go`: real `StartProcess` via `os/exec`, no build tags (starting
  a process needs no Linux-only syscall, so it compiles and genuinely runs
  on macOS too, exercised for real in `platform_test.go`).
- `backoff.go`: policy constants only (`DefaultBackoffBase`,
  `DefaultBackoffCap`, `StableAfter`) — the doubling/capping engine itself
  is the SHARED `cmd/gosd-init/internal/childbackoff` package (bean
  gosd-wxjy), imported rather than forked. The line-splitting relay writer
  is likewise the SHARED `cmd/gosd-init/internal/logwriter` package,
  imported in `tsfunnel.go`'s `runOnce` with prefix "tailscale-funnel: "
  (the gosd-o68e wiring bean's locked log-prefix choice, matching the
  --ingress flag value and gosd.toml section name rather than the shorter
  "tsfunnel" package/binary name).
- Full fake-driven test suite (`*_test.go`): mode-resolution table covering
  every failure row plus the happy path, funnel_port default-to-443, and
  hostname defaulting/override; `Run`'s network-up and time-synced gates
  (including Stop-closes-while-waiting and the time-synced-timeout warning);
  the state-dir preflight (created before the shim starts, and its EROFS
  path — one actionable line naming both `StateDir` and the data-partition
  requirement, with zero StartProcess calls); `supervise`/`runOnce`'s exact
  argv/env (including the empty-TS_AUTHKEY-once-state-exists case),
  escalating-backoff sequence, stable-reset, and network re-gate-before-
  restart; and an authkey-never-appears-in-any-log-output scan across every
  resolveMode failure mode plus a full runOnce with relayed stdout/stderr.
  `platform_test.go` exercises the real os/exec-backed StartProcess
  (env isolation from the parent process, missing-binary error).

Gates: `go test ./cmd/gosd-init/internal/tsfunnel/...` (including `-race`),
`go test ./...`, `go vet ./...`, `gofmt -l .`, and
`golangci-lint run --allow-parallel-runners ./...` both native and
`GOOS=linux` are all clean.
