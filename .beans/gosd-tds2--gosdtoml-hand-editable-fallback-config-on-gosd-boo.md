---
# gosd-tds2
title: 'gosd.toml: hand-editable fallback config on GOSD-BOOT'
status: in-progress
type: task
priority: normal
created_at: 2026-07-02T21:07:10Z
updated_at: 2026-07-04T12:26:43Z
parent: gosd-b22t
blocked_by:
    - gosd-kkz4
---

A human-editable TOML file at the root of the FAT partition, for non-Imager users and the Radxa (which has no WiFi and where Imager customization may not apply).

Schema v1 (locked, keep tiny): top-level `hostname = "..."`; `[wifi] ssid = "..."`, `passphrase = "..."`. Everything optional; missing file is fine. Parse with github.com/BurntSushi/toml.

The builder writes this file into every image with the build-time values (or commented-out examples when unset), so end users can open the SD card on any OS and edit it — comment header in the file explains exactly that in plain language for non-technical users.

- [x] Schema + parser + tests (valid, partial, garbage input — garbage logs a warning, never crashes)
- [x] Builder writes the template file; gosd-init reads it with top precedence
- [x] Comment header text reviewed for a non-technical audience

## Acceptance
Editing gosd.toml on the flashed card changes hostname/WiFi on next boot on both boards.

## Summary of Changes

Added `internal/gosdtoml`: the gosd.toml schema (`Config`/`Wifi`, mirroring initcfg's shape), `Parse` (BurntSushi/toml-backed; empty data is a no-op zero value, malformed TOML returns an error rather than panicking), and `Render` (writes the header plus hostname/[wifi] blocks, live values or commented-out examples).

`internal/pipeline.Assemble` now writes `gosd.toml` at the FAT root for every board (added after `Board.BootFiles` runs, not inside it), so pi-zero-2w and radxa-zero-3e both get it for free.

`cmd/gosd-init/internal/boot.Run` reads `/boot/gosd.toml` right after the boot-partition mount (step 5) via a new optional `Deps.ReadGosdToml`; a non-empty gosd.toml hostname overrides config.json's and the hostname is re-applied (`SetHostname` + log) before `/app` starts. Parse/read failures are logged as a warning and fall back to config.json — never fatal. The parsed gosd.toml is also passed through to `StartNetworking` (signature gained a `gosdtoml.Config` parameter).

`cmd/gosd-init/internal/wifiup.ConfigCredentials` gained a `GosdToml gosdtoml.Wifi` field: when its SSID is set it's used in place of the config.json wifi block entirely (same PSK/hex/open resolution logic), otherwise config.json's wifi is used unchanged. wifiup itself and its CredentialSource interface are untouched.

`cmd/gosd-init/main.go` wires a `readGosdToml` (missing file is silent, malformed TOML surfaces as an error) and threads gosd.toml's wifi block into `wifiupDeps`.

New dependency: github.com/BurntSushi/toml.

Deviations / notes:
- Went with a new `internal/gosdtoml` package rather than extending `internal/initcfg`, since the two config sources (JSON vs hand-edited TOML) have different failure semantics (config.json is baked and assumed present; gosd.toml is optional and hand-edited) even though the schemas rhyme.
- The bean's acceptance criterion (editing gosd.toml on a flashed card changes behavior on real hardware) is untested here — that requires physical boards and is left unchecked; everything else is verified by unit tests, `go vet`, `gofmt`, both golangci-lint views, and a static `CGO_ENABLED=0 GOOS=linux GOARCH=arm64` build of gosd-init.
