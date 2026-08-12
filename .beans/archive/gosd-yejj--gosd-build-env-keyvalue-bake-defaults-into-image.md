---
# gosd-yejj
title: 'gosd build --env KEY=VALUE: bake defaults into image'
status: completed
type: task
priority: normal
created_at: 2026-07-08T03:28:43Z
updated_at: 2026-07-09T10:06:29Z
parent: gosd-9b5c
blocked_by:
    - gosd-xnbd
---

Developer-facing build flag mirroring --wifi-ssid/--wifi-pass.
- `--env KEY=VALUE` (repeatable) on `gosd build`; validate KEY (POSIX-ish: [A-Za-z_][A-Za-z0-9_]*) and reject GOSD_* with an actionable error ("GOSD_* is reserved; choose another name"). Reject malformed entries (no =) actionably.
- Bake the parsed map into config.json (via initcfg Env from T2) AND render the same values into the generated gosd.toml [env] section (via T1s template) so the user sees and can override them on the card.
- Integration test (fake-artifacts): build with a couple of --env flags, read the image back, assert config.json carries them and gosd.toml [env] shows them; assert --env GOSD_FOO fails actionably.
- [x] flag + validation
- [x] bake to config.json + render into gosd.toml
- [x] integration test

## Summary of Changes

- Added a repeatable `--env KEY=VALUE` flag to `gosd build` (cmd/gosd/build.go), parsed by
  `parseEnvFlags`: KEY must match `[A-Za-z_][A-Za-z0-9_]*`, an entry missing `=` is rejected
  ("--env needs KEY=VALUE; got %q"), a KEY in the `GOSD_*` namespace is rejected, and a
  duplicate KEY across repeated --env flags is rejected outright (explicit rejection, not
  last-wins, to avoid a silently-shadowed typo). VALUE may be empty and is split on the first
  `=` only, so it may itself contain `=`.
- Added `Env map[string]string` to `boards.BuildConfig` (internal/boards/boards.go) and
  threaded it from cmd/gosd through `pipeline.Options.Config` into both sinks in
  internal/pipeline/pipeline.go: `initcfg.Config.Env` (config.json) and
  `gosdtoml.Render`'s env argument (gosd.toml [env], replacing the previously-nil value).
- Unit tests for parseEnvFlags (valid, missing =, bad KEY chars, reserved GOSD_, empty value,
  value containing =, duplicate key) in cmd/gosd/build_test.go; a pipeline-level unit test
  (internal/pipeline/pipeline_test.go) and CLI-level fake-artifacts integration tests
  (cmd/gosd/build_integration_test.go) confirming --env values land in both config.json and
  the rendered gosd.toml, and that --env GOSD_FOO=bar fails actionably.
