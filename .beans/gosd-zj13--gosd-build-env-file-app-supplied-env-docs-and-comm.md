---
# gosd-zj13
title: 'gosd build --env-file: app-supplied [env] docs and commented-out suggestions'
status: in-progress
type: feature
priority: normal
created_at: 2026-08-09T02:46:52Z
updated_at: 2026-08-09T02:46:52Z
---

## Context

Today an app developer can seed `[env]` **values** via repeatable `gosd build
--env KEY=VALUE`, but they get no way to *document* those keys for the end user:
`internal/gosdtoml/template.go`'s `Render()` emits one shared boilerplate
paragraph (`envHeader`) followed by bare, alphabetically-sorted `KEY = "value"`
lines (`template.go:226`). There is no per-key comment and no way to ship a
*suggested-but-inactive* variable. The schema is documented as "v1, locked"
(`internal/gosdtoml/config.go:1`), but that lock is about what the **runtime
parser** accepts on-card — it says nothing about how richly the build step may
*author* the initial file. This bean adds author-time richness without touching
the runtime schema.

JP wants a freshly-flashed card's `gosd.toml` to be able to look like this:

```toml
[env]
# uncomment this if you want the demo to run
# RUN_DEMO = "true"

# Where telemetry is posted; leave blank to disable
API_URL = "https://example.com"
```

i.e. per-key explanatory comments **and** commented-out suggestions the end user
opts into by uncommenting.

## Locked decisions

> **Design revised 2026-08-09 (JP): verbatim splice, not a structured
> manifest.** The first cut used a `[[env]]` array-of-tables manifest
> (`key`/`value`/`comment`/`suggested`); JP found the "TOML describing TOML"
> shape obtuse and asked instead for a plain TOML file transplanted into `[env]`
> verbatim. The decisions below reflect the delivered design; the manifest
> approach is retired.

- **Authoring surface: a verbatim `[env]`-body file** passed via
  `gosd build --env-file <path>` (single path). The file's contents *are* the
  card's `[env]` section, spliced in unchanged — the developer writes the
  comments, blank lines, active `KEY = "value"` entries and commented-out
  "suggested" entries exactly as they should appear. No structured schema to
  learn; the example in Context is literally a valid `--env-file`.
- **The file is the section body, no headers.** It carries only top-level
  `KEY = value` pairs and comments — no `[env]` header of its own (gosd frames
  it) and no other TOML section. JP's build steps: (1) parse it to confirm it's
  valid TOML with **no subheadings**; (2) render the rest of gosd.toml; (3) drop
  the file's text into the `[env]` section verbatim. Implemented as
  `gosdtoml.ParseEnvBody` (decode standalone; reject any table / array-of-tables
  as a section header) + `gosdtoml.EnvSection{Verbatim}` (splice under a bare
  `[env]`, no generic preamble).
- **Validation, so a bad file fails the build, not the boot:** valid TOML, no
  section headers, and every *active* key matches `^[A-Za-z_][A-Za-z0-9_]*$`
  with no `GOSD_*` prefix (same rules as `--env`). Actionable errors.
- **Active entries are still baked into `config.json`** (`initcfg.Config.Env`),
  so a default set here survives the user deleting `gosd.toml`, exactly like
  `--env`. Commented-out ("suggested") entries don't parse, so they're never
  baked — they do nothing until the user uncomments them, at which point
  `mergeUserEnv` (`cmd/gosd-init/internal/boot/sequence.go:500`) applies them
  from the card like any other hand-edit. Build-side only; no build→runtime
  contract change.
- **Verbatim means the developer owns quoting.** No forced re-quoting. An
  *active* bare scalar (`PORT = 8080`) is coerced to a string with a warning
  (on the console at boot and at build time), matching the on-card parser, so
  the docs advise quoting values meant to stay strings; a commented-out
  suggestion is free text (`# RUN_DEMO = true` is fine, per JP's example).
- **`--env` and `--env-file` are mutually exclusive** — the file is the whole
  section, so combining them is rejected with an actionable error rather than
  defining a merge.

## Known limitation (accepted for this bean; follow-up noted)

`gosdtoml.Render` is also called device-side by provsnapshot
(`cmd/gosd-init/internal/provsnapshot/provsnapshot.go:360,648`) when the
provisioning snapshot self-heals `gosd.toml` after a reflash. That re-render
works from parsed **values** (`map[string]string`, no comments — TOML comments
are discarded on parse), so after a reflash-and-self-heal the developer's
annotations won't reappear (env **values** are preserved; only the cosmetic
comments/commented-out formatting are lost). This is acceptable: it only affects
post-reflash cosmetics, never behaviour. If annotation-survival is later wanted,
a follow-up bean would carry the ordered `[]EnvEntry` (comment + suggested)
through `initcfg` into `config.json` and teach provsnapshot's merge to preserve
it while recomputing active/suggested from the effective env.

## Design / files

> **Superseded** by the verbatim-splice revision — see the note under Locked
> decisions and the Summary of Changes for what actually shipped. The
> `EnvEntry`/structured-manifest details below describe the retired first cut and
> are kept only for context.

- `internal/gosdtoml/`: new exported type
  ```go
  type EnvEntry struct {
      Key, Value, Comment string
      Suggested           bool
  }
  ```
  Change `Render`'s env parameter from `map[string]string` to `[]EnvEntry`
  (order-significant). Add `EnvEntriesFromMap(map[string]string) []EnvEntry`
  (sorted keys, no comments, all active) so the two provsnapshot callers keep
  today's exact plain output. Rendering helper splits multi-line comments into
  `# ` lines and quotes values.
- New `internal/gosdtoml` (or `internal/envmanifest`) parser: read the TOML
  manifest → `[]EnvEntry`, strict-decode, validate keys, reject `GOSD_*` and
  duplicate keys, actionable errors ("env-file entry #3: key \"…\" …").
- `cmd/gosd/build.go`: add `--env-file`; parse manifest; merge with `--env`
  flags per the rules above → the `[]EnvEntry` handed to the build and the
  active-only `map[string]string` baked into `initcfg.Config.Env`. Update the
  `--env` flag help to cross-reference `--env-file`.
- `internal/pipeline/pipeline.go:251`: pass the merged `[]EnvEntry` to `Render`.
- provsnapshot callers (`:360`, `:648`): wrap their `merged.Env` /
  `s.Effective.Env` with `EnvEntriesFromMap(...)`.
- Docs: extend the `[env]` documentation (added by bean gosd-eptf; check
  `docs/provisioning-formats.md`) with an `--env-file` section — the manifest
  format, the worked example above, the values-are-quoted note, and the
  reflash-self-heal limitation. No COMPATIBILITY.md change (host-side build
  feature).

## Verification

- Unit — manifest parse: valid file; errors for missing/invalid key, `GOSD_*`,
  duplicate key, unknown field, malformed TOML.
- Unit — render golden: active+comment, suggested+comment, multi-line comment,
  empty-value active, declaration order preserved, blank-line separation, values
  always quoted. Plus a guard that `EnvEntriesFromMap` round-trips to today's
  exact plain rendering (protects the provsnapshot callers).
- Unit — merge: `--env` overrides a manifest key's value and forces it active;
  `--env`-only key appended sorted; command-line precedence.
- Unit — baked defaults: suggested entries excluded from `initcfg.Config.Env`,
  active entries included.
- Integration (`cmd/gosd/build_integration_test.go` pattern): build with
  `--env-file` → read the image back → assert the documented/commented block is
  present in `/gosd.toml`; network-tripwire stays green.
- Full quality gates per CLAUDE.md before the PR.

## Todos

- [x] `EnvEntry` type + `Render([]EnvEntry, …)` signature change + multi-line
      comment / quoted-value rendering; `EnvEntriesFromMap` helper.
- [x] Update provsnapshot's two `Render` callers via `EnvEntriesFromMap`.
- [x] Env-manifest TOML parser + strict decode + validation with actionable
      errors.
- [x] `--env-file` flag, manifest↔`--env` merge, active-only baked map, `--env`
      help cross-reference.
- [x] Pipeline splices the verbatim `[env]` body via `Render`.
- [x] Unit tests (ParseEnvBody, verbatim render, envfile parse/validate) +
      integration test.
- [x] Docs: `--env-file` section with the worked example + quoting note +
      reflash-self-heal limitation.
- [x] Quality gates green; PR with bean status/todos updated.

## Summary of Changes

Added `gosd build --env-file <path>`: a plain TOML file whose contents become the
card's `gosd.toml [env]` section **verbatim** — the developer writes the section
(comments, active entries, commented-out "suggested" entries) exactly as it
should appear. Build-side only — no build→runtime contract change.

- `internal/gosdtoml`: `Render`'s env parameter is now an `EnvSection` — either
  a `Verbatim` body (spliced under a bare `[env]`, no generic preamble) or plain
  `Values` (the sorted `KEY = "value"` rendering used by `--env` and
  provsnapshot); empty renders the commented example. New `ParseEnvBody` decodes
  a standalone env body, **rejects any section header** (its own `[env]`, a stray
  `[wifi]`, `[[x]]`), and returns the active scalar entries + coercion warnings.
- `cmd/gosd`: `envfile.go`'s `parseEnvFile` returns the verbatim body + active
  defaults + warnings; validates via `ParseEnvBody` and the `--env` key rules;
  `--env`/`--env-file` are mutually exclusive. Active entries bake into
  `config.json`; commented-out ones don't parse, so they're never baked.
- `internal/pipeline`: `Options.EnvBody` carries the verbatim body;
  `Render` gets `EnvSection{Values: Config.Env, Verbatim: EnvBody}`.
- `cmd/gosd-init/internal/provsnapshot`: its two `Render` callers pass
  `EnvSection{Values: ...}` (identical output to before).
- Docs: new `docs/gosd.toml.md` (the file-format + `--env-file` authoring
  reference), linked from `README.md` and `docs/runtime.md`.

The verbatim design keeps JP's example working exactly as written, including the
unquoted `# RUN_DEMO = true` suggestion (docs advise quoting *active* values to
avoid the on-card bare-scalar warning). Accepted limitation recorded in the docs:
comments don't survive a reflash+snapshot self-heal (values do).
