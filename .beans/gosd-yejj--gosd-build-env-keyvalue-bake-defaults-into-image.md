---
# gosd-yejj
title: 'gosd build --env KEY=VALUE: bake defaults into image'
status: todo
type: task
created_at: 2026-07-08T03:28:43Z
updated_at: 2026-07-08T03:28:43Z
parent: gosd-9b5c
blocked_by:
    - gosd-xnbd
---

Developer-facing build flag mirroring --wifi-ssid/--wifi-pass.
- `--env KEY=VALUE` (repeatable) on `gosd build`; validate KEY (POSIX-ish: [A-Za-z_][A-Za-z0-9_]*) and reject GOSD_* with an actionable error ("GOSD_* is reserved; choose another name"). Reject malformed entries (no =) actionably.
- Bake the parsed map into config.json (via initcfg Env from T2) AND render the same values into the generated gosd.toml [env] section (via T1s template) so the user sees and can override them on the card.
- Integration test (fake-artifacts): build with a couple of --env flags, read the image back, assert config.json carries them and gosd.toml [env] shows them; assert --env GOSD_FOO fails actionably.
- [ ] flag + validation
- [ ] bake to config.json + render into gosd.toml
- [ ] integration test
