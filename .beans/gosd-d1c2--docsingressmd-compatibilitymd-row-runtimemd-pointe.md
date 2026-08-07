---
# gosd-d1c2
title: docs/ingress.md + COMPATIBILITY.md row + runtime.md pointer
status: todo
type: task
priority: normal
created_at: 2026-08-07T12:52:39Z
updated_at: 2026-08-07T12:53:22Z
parent: gosd-virc
blocked_by:
    - gosd-66ax
---

Ingress epic gosd-virc bean 6 (after the wiring bean lands).

## Content (locked)

- docs/ingress.md runbook: CLI-create the tunnel (`cloudflared tunnel login` →
  `tunnel create <name>` → `tunnel token <name>` → paste token + hostname +
  port into gosd.toml → `tunnel route dns <name> <hostname>`). State WHY
  CLI-created: dashboard-created tunnels are remote-managed and may override
  local ingress rules.
- Secrets-on-FAT note: the token sits plaintext on GOSD-BOOT, same trust level
  as the WiFi PSK in gosd.toml today.
- Clock/TLS note (no RTC on Pis; RTC epic reference), pinned-version/update
  story (--no-autoupdate; binary updates only via gosd release + reflash),
  metrics-port note (localhost-only listener cloudflared opens itself).
- COMPATIBILITY.md: feature row — arm64 boards ✅ (until bench: footnote
  "not yet hardware-verified"), pi-zero-w ❌ footnote (GOARM=7 asset faults on
  armv6; revisit if upstream ships v6). runtime.md gains a short pointer.
