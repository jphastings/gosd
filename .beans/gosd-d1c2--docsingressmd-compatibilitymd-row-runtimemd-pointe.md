---
# gosd-d1c2
title: docs/ingress.md + COMPATIBILITY.md row + runtime.md pointer
status: completed
type: task
priority: normal
created_at: 2026-08-07T12:52:39Z
updated_at: 2026-08-07T19:15:24Z
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

## Todos

- [x] `docs/ingress.md`: CLI-create runbook (`tunnel login` → `tunnel create` → `tunnel token` → paste into `gosd.toml` → `tunnel route dns`), why CLI-created (dashboard tunnels are remote-managed, bench characterization pending gosd-igk0), secrets-on-FAT note, clock/TLS note (RTC epic gosd-achn reference), pinned-version/update story (`--no-autoupdate`; update via a new `gosd` release + rebuild + reflash, no in-place path), the localhost-only metrics listener cloudflared opens itself, no-credentials-file/reflash-survival note, and a troubleshooting table quoting `mode.go`'s actual log lines verbatim
- [x] `COMPATIBILITY.md`: `--ingress cloudflared` feature row (arm64 boards ✅, footnote "not yet hardware-verified, bench bean gosd-igk0"; pi-zero-w ❌, footnote on the GOARM=7/armv6 asset fault) plus an overview bullet
- [x] `docs/runtime.md`: short pointer subsection to `docs/ingress.md`, and fixed the stale "what does not come back" line (gosd-tgzo's note) — `[ingress.cloudflared]` now listed as surviving a reflash, restored as a whole section like WiFi
- [x] Quality gates + PR

## Summary of Changes

- `docs/ingress.md` (new): the full ingress runbook and reference — CLI-tunnel-creation steps, why a CLI-created tunnel is required (dashboard/remote-managed tunnels aren't supported and haven't been bench-characterized against GoSD's locally-managed config.yml yet), the exact runtime file layout (`/run/gosd/cloudflared/{credentials.json,config.yml}`), why there's no credentials file on `GOSD-BOOT` (the token IS the triple), the FAT-plaintext secrets note (same trust level as the WiFi PSK), the clock/TLS startup window (2-minute time-synced gate then start-anyway, RTC epic gosd-achn noted as a future improvement, not a dependency), the pinned-version/no-autoupdate update story, the cloudflared-owned localhost metrics listener, reflash survival via the provisioning snapshot, a troubleshooting table quoting every `resolveMode` log line verbatim from `cmd/gosd-init/internal/cloudflared/mode.go`, and a "not supported yet" section (remote tunnels, multi-hostname, pi-zero-w).
- `COMPATIBILITY.md`: added an overview bullet and a `Board-specific features` row for Cloudflare Tunnel ingress — ✅ on every arm64 board (footnote: code-complete/unit/QEMU-tested, hardware verification pending bench bean `gosd-igk0`), ❌ on Pi Zero W (footnote: cloudflared's official `arm` release is GOARM=7 and faults on armv6).
- `docs/runtime.md`: added a short "Ingress: reaching your app from the internet" pointer subsection (under the HTTPS/networking section) linking to `docs/ingress.md`; fixed the provisioning-snapshot section's stale "what does **not** come back" line — `[ingress.cloudflared]` now survives a reflash too (bean `gosd-tgzo`, merged as PR #213), restored as a whole section rather than field-by-field since there's no baked default to diff against; also noted this in the restore-mechanics list and the "practical effect" paragraph.
- `.beans/gosd-f352--*.md` (new, separate commit): planning artifact for a low-priority CI-flakiness bug noticed in passing, per instructions — carried along, not otherwise related to this bean's work.

**Note for JP**: this branch (`bean/gosd-66ax-wire-cloudflared`, stacked on `gosd-g4km`/`gosd-uj36`/`gosd-7upw`) forked from `main` right after PR #208 merged and never rebased. Bean `gosd-tgzo`'s provsnapshot-restore work (PR #213) was, instead, branched directly off `main` after #208 and already merged there — so it is **not** present in this stack's `cmd/gosd-init/internal/provsnapshot/provsnapshot.go`, even though `docs/ingress.md`/`docs/runtime.md` here (correctly, per the epic's locked decisions and gosd-tgzo's own completed status) describe the finished, ingress-survives-reflash behavior. `main` already has both halves; the mismatch only exists on this unmerged stack and will need reconciling (a rebase, or letting the eventual merge sort it out) before `provsnapshot.go`'s code and these docs agree on this branch. Flagging so it isn't a surprise in review.

Gates: `gofmt -l .` clean; `go vet ./...` clean; `golangci-lint run --allow-parallel-runners ./...` and `GOOS=linux golangci-lint run --allow-parallel-runners ./...` both 0 issues. `go test ./...` hit severe shared-disk ENOSPC on this sandbox (~500Mi free system-wide, unrelated to this docs-only diff — no `.go` files changed); the directly-relevant packages (`internal/gosdtoml`, `cmd/gosd-init/internal/provsnapshot`, `cmd/gosd-init/internal/cloudflared`) passed before the disk filled up. CI is authoritative for the full suite.
