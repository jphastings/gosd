---
# gosd-r0be
title: 'gosd-init: merge, inject, and protect the app env'
status: todo
type: task
created_at: 2026-07-08T03:28:43Z
updated_at: 2026-07-08T03:28:43Z
parent: gosd-9b5c
blocked_by:
    - gosd-xnbd
---

Wire user env into the app launch.
- Add baked `Env map[string]string` to internal/initcfg Config (config.json) so developer --env defaults have a home.
- In cmd/gosd-init/internal/boot/sequence.go (~line 221 where env []string is built): after the GOSD_* vars, merge baked config.json env then gosd.toml [env] with PER-KEY precedence (gosd.toml wins). Drop any key in the reserved GOSD_* namespace with a log line. Append the survivors to the app env slice.
- Log keys+source only (never values), and log each gosdtoml parse warning surfaced by T1, and each reserved-key rejection.
- Fake-driven tests in the boot package: baked-only, card-only, per-key override, reserved-key rejection, coercion-warning logged, empty everywhere. Assert the final env slice the AppStarter receives (there is already a seam around deps.AppStarter.Start).
- [ ] initcfg baked Env field
- [ ] merge+precedence+reserved-protection in sequence.go
- [ ] key/source logging (no values)
- [ ] fake-driven tests
