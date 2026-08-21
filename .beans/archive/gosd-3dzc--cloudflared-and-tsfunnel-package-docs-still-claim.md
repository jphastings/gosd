---
# gosd-3dzc
title: cloudflared and tsfunnel package docs still claim to ship UNWIRED — both are live in main.go
status: completed
type: bug
priority: normal
created_at: 2026-08-16T04:43:32Z
updated_at: 2026-08-20T06:47:44Z
---

**Severity: Medium.** No runtime effect — this is a documentation-only bug —
but it actively misleads whoever reads the package doc to answer "is this
feature live," which is exactly the question a serial-console-only,
no-remote-debug device makes hardest to answer any other way.

## Verified

Both package docs open with the same claim:

```
cmd/gosd-init/internal/cloudflared/cloudflared.go:28:
// This module ships UNWIRED: nothing in cmd/gosd-init/main.go calls Run

cmd/gosd-init/internal/tsfunnel/tsfunnel.go:34:
// This module ships UNWIRED: nothing in cmd/gosd-init/main.go calls Run
```

Both are false today. `cmd/gosd-init/main.go` wires both in:

```
main.go:221:  cloudflared.Run(cloudflaredDeps(...), cloudflared.Options{...
main.go:237:  tsfunnel.Run(tsfunnelDeps(...), tsfunnel.Options{...
```

with full `cloudflaredDeps`/`tsfunnelDeps` wiring functions at `main.go:608`
and further down, plus `cloudflaredConfig`/`tsfunnelConfig` config-tree
readers at `main.go:520`/`main.go:530`. The archived beans `gosd-uj36`
("runtime module: cmd/gosd-init/internal/cloudflared — unwire[d]...") and
`gosd-e3mm` (the tsfunnel equivalent) confirm this was a deliberate staged
rollout: each module shipped unwired first, then a later bean wired it in.
The doc comments were never updated after that second step landed.

## Fix

Update both package docs to describe current reality: each module is started
from its own goroutine in `main.go`'s boot sequence, immediately before `/app`
supervision begins (per `StartNetworking`'s contract in
`cmd/gosd-init/internal/boot/sequence.go`), guarded by `PanicGuard`. Keep
whatever forward-looking caveats are still true (e.g. `--ingress` gating,
per-board support) but drop the "ships unwired" framing entirely.

## Todos

- [x] Rewrite the doc comment in `cloudflared.go:26-29`
- [x] Rewrite the doc comment in `tsfunnel.go:32-37`
- [x] Grep both files (and `docs/ingress.md`) for any other leftover
      "not yet wired" / "unwired" language from the staged rollout

## Summary of Changes

Fixed as part of the gosd-wnyc / gosd-5lz2 hardening batch (same PR) —
gosd-wnyc's ingress section carried the identical item, so it was done once
and ticked in both rather than left open twice.

Both package docs now open with what main.go actually does: `Run` is started
on its own PanicGuard-ed goroutine during StartNetworking, immediately before
/app supervision, with `Deps.StartProcess` and `Deps.Wait` wired from
`cloudflaredDeps`/`tsfunnelDeps` — and the caveat that is still true (whether
the tunnel or shim starts at all is `Options.Baked` plus the card's settings,
resolved by `resolveMode`) is kept.

The sweep for leftover staged-rollout tense found four more references to
"the later wiring bean" — `Deps`' own doc comment and `Options.Baked` in each
package, plus `tsfunnel.Options.BinaryPath` — all now naming the real wiring
functions instead of a future one. `docs/ingress.md` had no such language.
