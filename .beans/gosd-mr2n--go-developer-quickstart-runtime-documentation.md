---
# gosd-mr2n
title: Go developer quickstart + runtime documentation
status: in-progress
type: task
priority: normal
created_at: 2026-07-02T21:10:00Z
updated_at: 2026-07-04T12:27:30Z
parent: gosd-y0x3
---

Expand README.md (keep the existing GoSD intro) + docs/ for the Go-developer audience.

Content:
- [x] Quickstart: install (`go install github.com/jphastings/gosd/cmd/gosd@latest`), 10-line main.go, `gosd build`, flash, open http://hostname.local — written, but honestly caveated: no release is tagged yet, and `gosd build`'s image-assembly step is not yet implemented on `main` (see finding below), so the flow isn't runnable end to end today
- [x] Runtime contract page: docs/runtime.md — supervision, env vars, async networking, clock/SNTP, RAM storage + read-only /boot, serial logging, CGO/arm64 constraints. Also documents (grounded in current netup.go) that WiFi credentials are baked in by the CLI but gosd-init only brings up wired Ethernet today — see finding below
- [x] GPIO/I2C/SPI pointers (go-gpiocdev, periph.io) with the note that full examples land in v0.3 — in docs/runtime.md
- [x] Comparison note: when to use gokrazy instead — in docs/runtime.md
- [x] No volatile facts (timings, counts) included — qualitative statements + pointers to source files/commands throughout

## Acceptance
A Go developer with no embedded experience gets examples/hello running on a Pi Zero 2W using only these docs.

## Note from gosd-vtce (Ethernet networking)

gosd-init cannot set `GOSD_IP` as an env var: DHCP completes asynchronously, after /app has already been launched (networking must never block app start). When this quickstart/runtime doc is written, document that the app must discover its own address at runtime — e.g. via Go's `net.Interfaces()`/`net.InterfaceAddrs()` — rather than expecting an env var. `/run/gosd/network-up` (an empty marker file) exists as a signal that an address has been assigned, if polling for that is useful.

## Acceptance status

Not fully met yet, and can't be: the acceptance criterion ("a Go developer gets examples/hello running on a Pi Zero 2W using only these docs") requires real hardware and a working build pipeline. On `main` today, `gosd build`'s image-assembly step is a stub (`image.NotImplemented`, see `internal/image/assembler.go`) that always returns `errNotImplemented` — verified by actually running `go run ./cmd/gosd build ./examples/hello --board=pi-zero-2w -o /tmp/x.img`, which fails with `image assembly not implemented`. No image can be produced, let alone flashed, until gosd-3zrc (board profile + end-to-end build wiring) and gosd-wtpa (artifact pipeline) land. The docs are written honestly against this: the quickstart says so up front rather than promising a working flash+boot flow. Leaving this bean in-progress rather than completed, since the acceptance item is genuinely blocked on other beans, not on missing docs work.

## Findings (code/doc mismatches spotted while researching — not fixed, per instructions)

1. **`--artifacts-dir` doesn't exist on `main`.** Bean gosd-wtpa's body and gosd-3zrc's body both describe an `--artifacts-dir` flag on `gosd build`. It's not in `cmd/gosd/build.go` — confirmed via `grep -rn artifacts-dir --include="*.go" .` (no hits) and `go run ./cmd/gosd build --help` (not listed). It's planned as part of gosd-3zrc, not yet implemented. Docs do not mention this flag.
2. **`gosd build` cannot currently produce an image at all.** `internal/image/assembler.go`'s `Assembler` is wired to `image.NotImplemented{}` in `cmd/gosd/build.go`, which always returns `errNotImplemented`. Confirmed by actually running the build against `examples/hello`. This is more fundamental than "artifacts aren't published" (gosd-wtpa) — even with local artifacts there's no assembly step yet (gosd-3zrc). The quickstart in README.md is written to be honest about this.
3. **WiFi credentials are accepted but never acted on.** `gosd build --wifi-ssid`/`--wifi-pass` bake credentials into `config.json` (`initcfg.Config.Wifi`), but `cmd/gosd-init/internal/netup/netup.go`'s `Run` only watches/brings up wired interfaces matching `eth*`/`end*`/`enp*`. Nothing in `gosd-init` reads `cfg.Wifi` or performs WiFi association — `dhcp.go`'s doc comment confirms `RunDHCP` is exported specifically so "a later WiFi bean" can call it once nl80211 association exists. docs/runtime.md documents this gap explicitly so app authors don't assume WiFi works today.
4. **mDNS (`<hostname>.local`) isn't implemented yet.** CLAUDE.md's locked decisions describe mDNS as gosd-init's only network listener, and gosd-r796 tracks building it, but there's no mDNS code anywhere in the repo yet (`grep -rl mdns` = no hits). The quickstart notes that `.local` resolution needs gosd-r796 and suggests a router-based fallback in the meantime.

## Summary of Changes

Wrote developer-facing documentation grounded in the current `main` codebase, not the aspirational bean descriptions:

- Expanded README.md with a Quickstart (install, minimal main.go, `gosd build`, flash/open), kept the existing intro/Features section, and linked to docs/runtime.md. The quickstart is explicit up front about what doesn't work yet (no tagged release; `gosd build`'s image-assembly step is a stub; mDNS isn't built) so it doesn't over-promise.
- Added docs/runtime.md: the runtime contract for app authors — supervision/restart behavior, the exact two env vars gosd-init sets (`GOSD_BOARD`, `GOSD_HOSTNAME`) and why there's no `GOSD_IP`, async network bring-up (wired-only today) and the `/run/gosd/network-up` marker, the epoch-clock/SNTP-not-yet-built situation, RAM-only storage with read-only `/boot`, serial-only logging, CGO/arm64 build constraints, GPIO/I2C/SPI library pointers (v0.3 examples), and a generous gokrazy comparison.
- No Go files, go.mod, or workflows touched. Verified `gofmt -l .` is empty and the Go snippet in the README quickstart compiles (checked in a throwaway scratch module, then deleted).
- Filed the four findings above rather than fixing code, per task scope.

## Note from gosd-c8oj (SNTP time sync)

SNTP time sync has now landed (`cmd/gosd-init/internal/timesync`), so docs/runtime.md's "Clock: starts at 1970 until SNTP lands" section (lines ~79-94) is now stale and should be rewritten when this docs task is next picked up. What changed:

- /run/gosd/time-synced (`timesync.DefaultTimeSyncedPath`) now exists and is created on the first successful NTP sync — the marker the old text said didn't exist yet.
- gosd-init waits for /run/gosd/network-up, then retries SNTP (github.com/beevik/ntp) with backoff until the first success, sets the clock via settimeofday, and re-syncs hourly afterward.
- config.json gained an optional `ntpServers` field (`initcfg.Config.NTPServers`), defaulting to pool.ntp.org (`timesync.DefaultServers`) when omitted.
- App authors should gate TLS/x509-dependent calls on /run/gosd/time-synced existing, or just retry on failure until it does — please make this the documented guidance in place of the old "not yet built" caveat.
