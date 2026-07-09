---
# gosd-xnbd
title: 'gosdtoml: [env] schema, parser coercion, and template section'
status: completed
type: task
priority: normal
created_at: 2026-07-08T03:28:43Z
updated_at: 2026-07-09T06:44:22Z
parent: gosd-9b5c
---

Foundational; blocks the injection and builder tasks.
- Add `Env map[string]string` to internal/gosdtoml Config (`toml:"env"`).
- Parse coercion per the locked rules: quoted strings pass through; bare scalar (int/float/bool) → its string form + a returned warning; array/table/datetime under [env] → skip that key + warning. The parser must surface warnings to the caller (extend the existing return shape or add a warnings slice) so gosd-init can log them; malformed [env] never fails the whole parse (whole-file-optional invariant holds).
- template.go: add an [env] section to the generated gosd.toml with a plain-language comment header (match the existing hostname/wifi header voice — for a semi-technical reader: "Extra settings your app reads. Add lines like NAME = \"value\"."). If baked env values exist, render them live; else show a commented example. 
- Tests: parse (strings, coercion+warning, skipped non-scalars, empty/missing), and exact-output render (with and without baked values).
- [x] Env field + coercion + warnings plumbed
- [x] Template [env] section + header
- [x] Parse + render tests

## Summary of Changes

- Added `Env map[string]string` (`toml:"env"`) to `gosdtoml.Config`.
- `Parse` now returns `(Config, []string, error)` — the middle slice is
  human-readable warnings, sorted by key, for coercions and drops under
  [env]. Decodes into an internal `rawConfig` with `Env map[string]any`
  first so a bare scalar (or any other type) never turns into a hard TOML
  decode error; `coerceEnv` does the coercion/drop/warn afterwards, so a
  bad [env] entry can never take hostname/wifi down with it.
- Bare int/float/bool values are coerced to their canonical string form
  (`fmt.Sprintf("%v", v)`) with a warning; arrays, inline tables and
  datetimes are dropped (key omitted from the map) with a warning. Missing
  or empty [env] yields a nil Env map and no warnings.
- template.go grew an [env] section mirroring the hostname/wifi voice:
  commented-out NAME/"value" example when no baked env is given, or a
  live, key-sorted [env] table when it is. `Render` gained a fourth
  `env map[string]string` parameter.
- Updated the two existing callers to keep the repo compiling:
  `internal/pipeline/pipeline.go` passes `nil` for baked env (that's
  `gosd build --env`'s job, bean gosd-yejj); `cmd/gosd-init/main.go`'s
  `readGosdToml` discards the new warnings return value for now (wiring
  it into boot logging is bean gosd-r0be's job) — neither file's own
  behaviour changed.
- Test comparisons switched from `==`/`!=` to `reflect.DeepEqual` since
  Config now holds a map and is no longer comparable with `==`.
