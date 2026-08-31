---
# gosd-3ye3
title: 'dhcpserver package: minimal DHCPv4 server for a gosd-hosted subnet'
status: todo
type: task
created_at: 2026-08-31T05:36:11Z
updated_at: 2026-08-31T05:36:11Z
parent: gosd-qfbk
---

WiFi AP epic gosd-qfbk bean 2. No dependency on the other child beans; blocks
bean 3 (wifiap composes this package).

## Locked decisions

- New package `cmd/gosd-init/internal/dhcpserver/`, wrapping
  `github.com/insomniacslk/dhcp/dhcpv4/server4` (already a direct go.mod
  dependency, subpackage currently unused — zero new dependency).
- Binds one named interface, serves a configured `(rangeStart, rangeEnd,
  serverIP, leaseTime)` — **range-based, not single-lease**, deliberately
  generic enough that bean gosd-30jz (USB Ethernet gadget) can reuse it
  later as a degenerate one-address case (`10.55.0.2`–`10.55.0.2`). This
  bean does not wire gosd-30jz itself — just don't design against reuse.
- In-memory MAC→IP lease table, no persistence (ephemeral like
  cloudflared's `/run` files — fully reconstructible every boot).
- Shape mirrors the rest of gosd-init: pure lease-assignment logic behind a
  small interface seam, fake-tested on macOS; the real socket bind (needs
  `CAP_NET_BIND_SERVICE`/root, which gosd-init as PID 1 already has) isolated
  in `platform_linux.go` with a `platform_other.go` stub.
- DHCP pool sizing default `.10`–`.200` proposed at the epic level (open
  question 6) — a genuinely multi-client subnet, unlike gosd-30jz's
  single-host USB case.

## Todos

- [ ] `dhcpserver` package: pure lease-assignment logic + `Deps` interface
- [ ] `platform_linux.go` real UDP/67 bind via `server4`; `platform_other.go`
      stub
- [ ] Fake-driven unit tests (lease assignment, exhausted pool, renewal,
      release) passing on macOS
- [ ] Doc comment recording the "designed for reuse by gosd-30jz" intent
