---
# gosd-jsqa
title: 'COMPATIBILITY.md: board × feature support matrix'
status: completed
type: task
priority: normal
created_at: 2026-07-06T07:15:17Z
updated_at: 2026-07-06T08:13:46Z
---

JP requested a board × feature compatibility matrix at the repo root, for Go developers evaluating GoSD: which boards support Ethernet, WiFi, imager provisioning, mDNS, SNTP, persistent storage, USB gadget, GPIO/I2C/SPI, OTA updates, etc, derived from actual repo state (code, kernel fragments, beans) rather than aspirational docs.



## Todos
- [x] Write COMPATIBILITY.md at repo root: board (Pi Zero 2W, Radxa Zero 3E, NanoPi Zero2) x feature matrix, every cell derived from repo state (internal/boards, cmd/gosd-init/internal/*, build/boards, docs, beans)
- [x] Footnotes: hardware bring-up caveat (prominent), Imager device-tag filtering nuance (Pi3 shared tag; non-Pi "No filtering" only), NanoPi USB kernel gate + U-Boot v2026.07 gate, hidden-SSID parsed-not-joinable, NanoPi FPC GPIO connector, qemu-virt internal-only italic mention
- [x] Link COMPATIBILITY.md from README.md near the boards mention
- [x] Add one line to CLAUDE.md docs/conventions area requiring COMPATIBILITY.md updates alongside board/feature status changes
- [x] Quality gates: go test ./..., go vet ./..., gofmt -l ., golangci-lint run ./... (macOS + GOOS=linux) — all clean
- [x] Rebase on origin/main, open PR (no self-merge)


## Summary of Changes

Added `COMPATIBILITY.md` at the repo root: a board x feature matrix (Raspberry
Pi Zero 2W, Radxa Zero 3E, NanoPi Zero2 planned; qemu-virt deliberately
excluded per CLAUDE's locked decision, mentioned only in a closing italic
line) covering image build, published artifacts, Ethernet, WiFi (incl. hidden
SSID), Imager catalog provisioning (incl. device-tag filtering), gosd.toml,
mDNS, SNTP, the /data partition, USB gadget, GPIO/I2C/SPI, and OTA updates.

Every cell was checked against actual repo state rather than bean text alone:
read internal/boards/* (confirmed Radxa has no WiFi driver/firmware, Pi has no
wired-Ethernet driver — both hardware facts, not gaps), grepped for
gpio/gadget code (none exists yet, confirming those rows are planned, not
just under-documented), checked cmd/gosd-init/main.go's wiring (netup, wifiup,
timesync, mdnsresponder are all live, contradicting docs/runtime.md's stale
"SNTP not yet built" section, so the matrix reflects code state over that
doc), checked build-artifacts.yml and kernel fragments for the NanoPi USB gate
and GPIO/I2C/SPI kernel config survival, and confirmed the artifacts/v0.1.0
tag exists (contradicting READMEs stale quickstart caveat, also not fixed
here as out of scope).

Linked from README.md near the boards mention; added the "COMPATIBILITY.md
must be updated alongside board/feature status changes" line to CLAUDE.md's
code-conventions section. Quality gates (go test, go vet, gofmt, golangci-lint
on both macOS and GOOS=linux) all clean — docs-only change.
