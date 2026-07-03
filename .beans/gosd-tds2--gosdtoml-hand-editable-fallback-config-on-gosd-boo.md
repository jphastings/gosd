---
# gosd-tds2
title: 'gosd.toml: hand-editable fallback config on GOSD-BOOT'
status: todo
type: task
priority: normal
created_at: 2026-07-02T21:07:10Z
updated_at: 2026-07-02T21:10:20Z
parent: gosd-b22t
blocked_by:
    - gosd-kkz4
---

A human-editable TOML file at the root of the FAT partition, for non-Imager users and the Radxa (which has no WiFi and where Imager customization may not apply).

Schema v1 (locked, keep tiny): top-level `hostname = "..."`; `[wifi] ssid = "..."`, `passphrase = "..."`. Everything optional; missing file is fine. Parse with github.com/BurntSushi/toml.

The builder writes this file into every image with the build-time values (or commented-out examples when unset), so end users can open the SD card on any OS and edit it — comment header in the file explains exactly that in plain language for non-technical users.

- [ ] Schema + parser + tests (valid, partial, garbage input — garbage logs a warning, never crashes)
- [ ] Builder writes the template file; gosd-init reads it with top precedence
- [ ] Comment header text reviewed for a non-technical audience

## Acceptance
Editing gosd.toml on the flashed card changes hostname/WiFi on next boot on both boards.
