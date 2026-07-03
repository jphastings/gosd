---
# gosd-mr2n
title: Go developer quickstart + runtime documentation
status: todo
type: task
priority: normal
created_at: 2026-07-02T21:10:00Z
updated_at: 2026-07-03T17:59:44Z
parent: gosd-y0x3
---

Expand README.md (keep the existing GoSD intro) + docs/ for the Go-developer audience.

Content:
- [ ] Quickstart: install (`go install github.com/jphastings/gosd/cmd/gosd@latest`), 10-line main.go, `gosd build`, flash, open http://hostname.local — under 5 minutes end to end
- [ ] Runtime contract page: your app is /app supervised by gosd-init; env vars (GOSD_BOARD, GOSD_HOSTNAME); network comes up async — retry, do not assume; clock starts at 1970 until SNTP (gate TLS on retry or /run/gosd/time-synced); everything is in RAM except /boot (ro) — no persistence until v0.3; logs go to serial console; CGO_ENABLED=0 only, arm64 only
- [ ] GPIO/I2C/SPI pointers (go-gpiocdev, periph.io) with the note that full examples land in v0.3
- [ ] Comparison note: when to use gokrazy instead (multi-service appliances, self-updating fleets) — be generous, we build on their ideas
- [ ] Do not include volatile facts (timings, counts) — qualitative statements + commands to check

## Acceptance
A Go developer with no embedded experience gets examples/hello running on a Pi Zero 2W using only these docs.

## Note from gosd-vtce (Ethernet networking)

gosd-init cannot set `GOSD_IP` as an env var: DHCP completes asynchronously, after /app has already been launched (networking must never block app start). When this quickstart/runtime doc is written, document that the app must discover its own address at runtime — e.g. via Go's `net.Interfaces()`/`net.InterfaceAddrs()` — rather than expecting an env var. `/run/gosd/network-up` (an empty marker file) exists as a signal that an address has been assigned, if polling for that is useful.
