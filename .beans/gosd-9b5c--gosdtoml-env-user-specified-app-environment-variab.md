---
# gosd-9b5c
title: 'gosd.toml [env]: user-specified app environment variables'
status: completed
type: feature
priority: normal
created_at: 2026-07-08T03:28:43Z
updated_at: 2026-07-09T11:44:32Z
---

Let whoever holds the SD card set environment variables that gosd-init injects into /app at boot, so one compiled appliance is configured per-deployment without a rebuild. Standard 12-factor env config, sourced from the boot partition.

SURFACE (locked, JP 2026-07-08): an [env] table in gosd.toml — NOT a separate file, NOT cloud-init. Reuses the existing hand-editable file, its friendly comment header, and its precedence chain.

LOCKED DECISIONS (do not relitigate):
- Precedence, highest wins PER KEY (env is a map, not a single value — merge, do not whole-map-replace):
  1. gosd.toml [env]  (on the card — the user edit)
  2. baked config.json env  (developer defaults via `gosd build --env`)
- Reserved namespace: GOSD_* is gosd-inits. A card [env] key matching GOSD_* (or exactly GOSD_BOARD/GOSD_HOSTNAME/GOSD_DATA) is logged-and-ignored, never applied. gosd-init still sets the real GOSD_* vars as today.
- App environment stays a clean slate (current behaviour): the app gets exactly GOSD_* + the merged user env, NOT os.Environ inheritance.
- TOML value coercion: [env] values should be quoted strings. A bare scalar (int/float/bool, e.g. PORT = 8080) is coerced to its string form with a one-line warning; arrays/tables/datetimes under [env] are skipped with a warning. Missing/empty [env] = no vars, no error.
- Logging: log the KEYS set and their source, NEVER values (may be secrets). e.g. "app env: API_URL, LOG_LEVEL (gosd.toml); PORT (baked)"; "ignoring reserved env key GOSD_FOO from gosd.toml".
- Security: env values live in plaintext on a FAT partition (same exposure as the WiFi PSK). Document, do not encrypt.

Consequence: no artifact release needed (all changes are in gosd-init + the builder + docs; no kernel/DTB/bootloader change).

## Summary of Changes
Shipped across four PRs (#53 schema/template, #54 gosd-init inject, #55 --env flag, #56 docs/example):
- internal/gosdtoml [env] table with scalar coercion + warnings; template section on the card.
- gosd-init merges baked config.json env with gosd.toml [env] per-key (card wins), protects GOSD_*, logs keys+source only.
- `gosd build --env KEY=VALUE` bakes defaults into both config.json and the rendered gosd.toml.
- docs/runtime.md + publishing.md + flashing.md + examples/hello (optional GREETING) + COMPATIBILITY row.
No artifact release needed (gosd-init + builder + docs only). Hardware-agnostic; runs on all four boards.
