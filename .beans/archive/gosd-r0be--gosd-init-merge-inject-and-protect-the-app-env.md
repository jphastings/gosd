---
# gosd-r0be
title: 'gosd-init: merge, inject, and protect the app env'
status: completed
type: task
priority: normal
created_at: 2026-07-08T03:28:43Z
updated_at: 2026-07-09T08:24:58Z
parent: gosd-9b5c
blocked_by:
    - gosd-xnbd
---

Wire user env into the app launch.
- Add baked `Env map[string]string` to internal/initcfg Config (config.json) so developer --env defaults have a home.
- In cmd/gosd-init/internal/boot/sequence.go (~line 221 where env []string is built): after the GOSD_* vars, merge baked config.json env then gosd.toml [env] with PER-KEY precedence (gosd.toml wins). Drop any key in the reserved GOSD_* namespace with a log line. Append the survivors to the app env slice.
- Log keys+source only (never values), and log each gosdtoml parse warning surfaced by T1, and each reserved-key rejection.
- Fake-driven tests in the boot package: baked-only, card-only, per-key override, reserved-key rejection, coercion-warning logged, empty everywhere. Assert the final env slice the AppStarter receives (there is already a seam around deps.AppStarter.Start).
- [x] initcfg baked Env field
- [x] merge+precedence+reserved-protection in sequence.go
- [x] key/source logging (no values)
- [x] fake-driven tests

## Summary of Changes

- `internal/initcfg.Config` gained `Env map[string]string` (json tag `env`,
  omitempty): the home for developer-baked env defaults (gosd-yejj's
  `--env` flag lands values here later).
- `boot.Deps.ReadGosdToml` now returns `(gosdtoml.Config, []string, error)`;
  `cmd/gosd-init/main.go`'s `readGosdToml` passes `gosdtoml.Parse`'s
  warnings straight through instead of discarding them, and `boot.Run` logs
  each one.
- `boot.Run` builds the app env as before (GOSD_BOARD, GOSD_HOSTNAME,
  optional GOSD_DATA), then appends the merged user env via a new
  `mergeUserEnv` helper: baked `config.json` env first, gosd.toml `[env]`
  overlaid per-key (gosd.toml wins), any key in the reserved `GOSD_*`
  namespace dropped with a per-key log line, survivors sorted for
  deterministic env ordering. A single summary line logs which keys came
  from gosd.toml vs baked (keys only, never values).
- Added fake-driven tests in `cmd/gosd-init/internal/boot/sequence_test.go`
  covering baked-only, card-only, per-key override, reserved-key rejection
  (including a non-GOSD_BOARD/-HOSTNAME/-DATA key), a gosd.toml parse
  warning being logged, and the unchanged-when-empty case.
