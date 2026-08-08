---
# gosd-o68e
title: Wire tsfunnel into StartNetworking + qemu smoke
status: completed
type: task
priority: normal
created_at: 2026-08-07T15:09:28Z
updated_at: 2026-08-07T15:09:35Z
parent: gosd-65uy
blocked_by:
    - gosd-kzd3
    - gosd-e3mm
    - gosd-66ax
---

Tailscale epic gosd-65uy bean 5 (after TS-3 gosd-kzd3 and TS-4 gosd-e3mm;
needs cloudflared's gosd-66ax — the gosd-oyhi supervision carve-out and the
plural-children reaper test land there and already cover a second child, so
NO further contract amendment here).

## Locked decisions

- Second guard.Go("tailscale-funnel", ...) in StartNetworking
  (cmd/gosd-init/main.go); tsfunnelDeps constructor beside cloudflaredDeps;
  no boot.Deps signature change (StartNetworking already receives cfg +
  gosdToml). Child stdout/stderr → two shared-logwriter instances, prefix
  "tailscale-funnel: ". Mind main.go's wifiup early-return ordering — the
  new guard.Go must start before any code path that returns on
  Ethernet-only boards.
- qemu CI via gosd run --ingress tailscale-funnel --data-size 64MiB
  (TS-3's new run flag; qemu-virt is arm64):
  - no [ingress.tailscale-funnel] section → EXACTLY the one quiet
    baked-but-unconfigured line;
  - dummy-authkey run → supervisor start line + shim nonzero exit + backoff
    line. This assertion stays LOOSE (or allowed-to-fail) until TS-8
    characterizes real invalid-key behavior — error vs hang bounded by
    --register-timeout — then it is tightened.

## Todos

[x] wiring + deps constructor
[x] qemu smoke: unconfigured quiet line
[ ] qemu smoke: dummy-key supervise cycle (loose; tighten after TS-8) —
    deliberately SKIPPED, not merely loosened; see Summary of Changes for
    why (mirrors gosd-66ax's cloudflared precedent, plus a structural
    blocker cloudflared didn't have)

## Summary of Changes

This branch necessarily merges the e3mm module lineage into the kzd3
build-rail lineage (`git merge origin/bean/gosd-e3mm-tsfunnel-module` onto
`origin/bean/gosd-kzd3-tsfunnel-build-rail`, clean, no conflicts) because
this bean's wiring needs both TS-3 (the build rail: `--ingress
tailscale-funnel`, per-arch shim compile, `--data-size` on `gosd run`) and
TS-4 (the unwired `cmd/gosd-init/internal/tsfunnel` runtime module) at once.
Only the commits after that merge are this bean's own.

- `cmd/gosd-init/main.go`: added `guard.Go("tailscale-funnel", ...)` inside
  `StartNetworking`, placed right after the `cloudflared` guard.Go call
  (still before the WiFi-hardware-absent early return, so an Ethernet-only
  board never skips it), plus the `tsfunnelDeps` constructor (StartProcess →
  `tsfunnel.StartProcess`, Wait → `platform.Reaper.Wait`, NetworkUp/
  TimeSynced → the same two marker checks `cloudflaredDeps` already wires,
  MkdirAll → `os.MkdirAll`, Clock → `tsfunnel.NewRealClock()` — already
  shipped with the e3mm module, unlike cloudflared which needed one added in
  gosd-66ax — NewBackoff → `childbackoff.NewBackoff(tsfunnel.
  DefaultBackoffBase, tsfunnel.DefaultBackoffCap)`) and the
  `tsfunnelBinaryPath` constant (`/bin/gosd-tsfunnel`, mirroring
  `cloudflaredBinaryPath`'s doc comment and duplicated for the same reason:
  `cmd/gosd/ingress.go`'s `ingressTailscaleFunnelDest` lives in a different
  binary's internal package). `tsfunnel.Options.Baked` comes from
  `cfg.IngressTailscaleFunnel`, `.Config` from `gosdToml.Ingress.
  TailscaleFunnel`, `.Hostname` from `cfg.Hostname` (the same field
  `mdnsresponderDeps` already wires). No `boot.Deps` signature change —
  `StartNetworking` already receives `cfg` and `gosdToml`.
- `cmd/gosd-init/internal/boot/reaper.go`: added "and tailscale-funnel" to
  the stash comment's "gosd-shipped system services like cloudflared"
  example list — the eviction argument already covered a second child
  (gosd-66ax's amendment), so no further contract change was needed here.
- `docs/runtime.md`: the two build-constraints bullets that named only
  `cloudflared` as gosd-init's gosd-shipped-service exception now name both
  agents — "whichever `--ingress` agent the image was built with
  (`cloudflared` or `tailscale-funnel` — both dial out rather than bind a
  socket on any real host interface, so neither adds a listener)" and
  "currently `cloudflared` and `tailscale-funnel`, each started only when an
  image is built with the matching `--ingress` value". `docs/ingress.md` and
  COMPATIBILITY.md are deliberately untouched — that's bean gosd-1cqa.
- `CLAUDE.md`: the "gosd-init has no interactive surface" locked decision
  gains a sentence for tailscale-funnel alongside cloudflared's existing
  one, explaining why tsnet's userspace netstack (dialing out over
  WireGuard, no socket bound on any real host interface) keeps the "no
  listeners" claim true even though Funnel makes the app publicly
  reachable.
- `.github/workflows/ci.yml`: one additive step in the `qemu-boot` job,
  directly after cloudflared's equivalent step and reusing the same
  `dist/gosd-cli` binary: `gosd run --ingress tailscale-funnel --data-size
  64MiB` (the `--data-size` is required — tailscale-funnel refuses to build
  with no GOSD-DATA partition, bean gosd-kzd3) with no
  `[ingress.tailscale-funnel]` section, asserting the "baked but not
  configured" line appears exactly once. actionlint clean.
  - The dummy-authkey supervisor-loop case (start line, nonzero exit,
    backoff line) is deliberately NOT implemented, for a stronger version of
    the reasons gosd-66ax gave for skipping cloudflared's equivalent case:
    it would need a real registration attempt against Tailscale's control
    plane (a genuine network expectation, plus pinning an upstream
    error-wording detail this repo doesn't control), AND — unlike
    cloudflared, which at least has no structural blocker — gosd.toml's
    ingress secrets are never baked at build/run time by design
    (`internal/pipeline/pipeline.go`'s "never a real value to bake here"),
    so reaching that code path in CI at all would require new image-editing
    tooling (e.g. mtools or a go-diskfs write against the built .img) that
    this bean does not add. This matches the locked decision's own "stays
    LOOSE (or allowed-to-fail)" allowance; real invalid-authkey behavior is
    bench-only, characterized by TS-8 (gosd-79v8).

Gates: targeted packages, then `go test ./...`, `go vet ./...`, `gofmt -l .`,
`golangci-lint run --allow-parallel-runners ./...` and
`GOOS=linux golangci-lint run --allow-parallel-runners ./...` (native run
needed one `golangci-lint cache clean` first — a stale sibling-worktree
false positive, per CLAUDE.md's documented gotcha).
