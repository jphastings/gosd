---
# gosd-o68e
title: Wire tsfunnel into StartNetworking + qemu smoke
status: todo
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

[ ] wiring + deps constructor
[ ] qemu smoke: unconfigured quiet line
[ ] qemu smoke: dummy-key supervise cycle (loose; tighten after TS-8)
