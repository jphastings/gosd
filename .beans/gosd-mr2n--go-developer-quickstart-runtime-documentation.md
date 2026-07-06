---
# gosd-mr2n
title: Go developer quickstart + runtime documentation
status: in-progress
type: task
priority: normal
created_at: 2026-07-02T21:10:00Z
updated_at: 2026-07-06T10:15:19Z
parent: gosd-y0x3
---

Expand README.md (keep the existing GoSD intro) + docs/ for the Go-developer audience.

Content:
- [x] Quickstart: install (`go install github.com/jphastings/gosd/cmd/gosd@latest`), 10-line main.go, `gosd build`, flash, open http://hostname.local — written; refreshed 2026-07-06 now that `artifacts/v0.1.0` is published and `gosd build` with no extra flags assembles a real, flashable .img end-to-end on a clean machine (verified). Only hardware bring-up (flashing + booting a real board) remains unconfirmed.
- [x] Runtime contract page: docs/runtime.md — supervision, env vars, async networking, clock/SNTP, RAM storage + read-only /boot, serial logging, CGO/arm64 constraints. Refreshed 2026-07-06: WiFi bring-up (wifiup, nl80211), mDNS, SNTP/time-synced marker, cloud-init+gosd.toml provisioning precedence, the GOSD_DATA partition, and USB gadget mode are all now implemented and documented as present-tense, not planned.
- [x] GPIO/I2C/SPI pointers (go-gpiocdev, periph.io) with the note that full examples land in v0.3 — in docs/runtime.md
- [x] Comparison note: when to use gokrazy instead — in docs/runtime.md
- [x] No volatile facts (timings, counts) included — qualitative statements + pointers to source files/commands throughout

## Acceptance
A Go developer with no embedded experience gets examples/hello running on a Pi Zero 2W using only these docs.

## Note from gosd-vtce (Ethernet networking)

gosd-init cannot set `GOSD_IP` as an env var: DHCP completes asynchronously, after /app has already been launched (networking must never block app start). When this quickstart/runtime doc is written, document that the app must discover its own address at runtime — e.g. via Go's `net.Interfaces()`/`net.InterfaceAddrs()` — rather than expecting an env var. `/run/gosd/network-up` (an empty marker file) exists as a signal that an address has been assigned, if polling for that is useful.

## Acceptance status

Updated 2026-07-06: gosd-3zrc and gosd-wtpa have since landed. `gosd build` now fully assembles a real, flashable .img — verified end-to-end on a clean machine (empty HOME/cache, downloads and sha256-verifies `artifacts/v0.1.0`, produces a bootable image; an immediate rebuild with networking killed succeeds from the cache alone). The build/install/assemble portion of the acceptance criterion is met. What remains is genuinely hardware-gated: no image has been flashed to or booted from a real Pi Zero 2W or Radxa Zero 3E yet (beans gosd-m9dj/gosd-nlzf, both blocked on acquiring a bring-up kit, gosd-s4t4). Leaving this bean in-progress rather than completed for that reason alone — the docs themselves (README.md, docs/runtime.md, COMPATIBILITY.md) are believed current as of this PR (bean/gosd-mr2n-docs-refresh).

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

## Docs refresh (bean/gosd-mr2n-docs-refresh, 2026-07-06)

Re-verified every stale claim against current `main` rather than trusting the earlier notes above, then fixed what had actually gone stale:

- **README.md quickstart**: rewrote the pre-release caveat block. `artifacts/v0.1.0` is published and confirmed (`gh release list`); `go install .../gosd@latest` works today via Go's pseudo-version resolution even with no tagged CLI release (verified with a real `go install` + `gosd build` run from a clean `$HOME`, producing a genuine `.img`). Removed the stale "mDNS not yet built" caveat on the `.local` step — mDNS is implemented and wired in `cmd/gosd-init/main.go`. The only remaining honest caveat is hardware bring-up (no image flashed/booted on a real board yet, beans `gosd-m9dj`/`gosd-nlzf`). Also linked the USB OTG feature bullet to `docs/runtime.md#usb-gadget-mode` and `examples/usbserial`.
- **docs/runtime.md**: rewrote the "Clock: starts at 1970 until SNTP lands" section (stale per gosd-c8oj's note above) to describe present-tense behavior — `/run/gosd/time-synced`, hourly re-sync, `ntpServers` config field. Updated the build-constraints bullet that still said mDNS/update listeners were "(later)" — mDNS runs today; only the update listener is still future.
- **COMPATIBILITY.md**: the USB-gadget row/footnote went stale within the same day it was written — PR #35 (bean `gosd-uo9f`) landed the `gadget` package immediately after gosd-jsqa wrote the compatibility matrix describing it as not-yet-written. Flipped the Pi/Radxa cells from 🚧 to ✅ (serial works; USB Ethernet, bean `gosd-30jz`, is still 🚧) and reworded the footnote, keeping the "library ≠ hardware-verified" caveat intact (bring-up beans `gosd-m9dj`/`gosd-nlzf` are still todo, blocked on `gosd-s4t4`).
- Swept README.md, docs/runtime.md, docs/publishing.md, docs/artifacts.md, docs/provisioning-formats.md for other "not yet"/"forthcoming"/"planned"/"TODO" language; everything else found (GPIO/I2C/SPI worked examples for v0.3, USB Ethernet gadget, OTA updates, the NanoPi Zero2 board profile, the screenshot flashing guide, bench-only provisioning-format todos) is genuinely still future work and was left as-is.

Quality gates (`go test ./...`, `go vet ./...`, `gofmt -l .`, `golangci-lint run ./...` on both host and `GOOS=linux`) all pass — this PR touches only Markdown.
