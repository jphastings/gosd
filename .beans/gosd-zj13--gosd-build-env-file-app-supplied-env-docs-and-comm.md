---
# gosd-zj13
title: 'gosd build --env-file: app-supplied [env] docs and commented-out suggestions'
status: todo
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

- **Authoring surface: a declarative env-manifest file** passed via a new
  `gosd build --env-file <path>` flag (JP, 2026-08-09; chosen over cramming
  comments into `--env` flags, and over a full free-text template that would
  fight the "safe for non-technical users to edit" schema intent). Single path;
  a repeated `--env-file` is an error.
- **Manifest format is TOML, array-of-tables**, consistent with `gosd.toml`
  itself and the existing TOML dependency. One `[[env]]` table per variable:
  - `key`   (string, **required**; validated `^[A-Za-z_][A-Za-z0-9_]*$`, `GOSD_*`
             rejected — same rules as `--env`, see `cmd/gosd/build.go:551,577`)
  - `value` (string, optional, default `""`)
  - `comment` (string, optional; a `\n` splits it into multiple `# ` lines)
  - `suggested` (bool, optional, default `false`)
  Decode **strictly** (unknown fields → error): this is a developer-facing build
  input, so fail fast on typos like `commnet =`, unlike the deliberately-lenient
  runtime `gosd.toml` parser.
- **Rendering:**
  - active entry (`suggested=false`): comment lines (if any), then
    `KEY = "value"`.
  - suggested entry (`suggested=true`): comment lines (if any), then
    `# KEY = "value"` (the whole assignment commented out).
  - one blank line separating consecutive documented entries; the generic
    `envHeader` paragraph is **kept** above them (it still orients a
    non-technical reader).
- **Values render quoted (`%q`) always** — including inside the commented
  suggestion (`# RUN_DEMO = "true"`, not `# RUN_DEMO = true`). This is a
  deliberate one-character deviation from JP's example: an unquoted `[env]`
  scalar parses but emits a boot-time console warning ("bare bool, not a quoted
  string; add quotes to silence", `internal/gosdtoml/config.go:258`), so the
  uncomment path must land warning-free.
- **Merge with `--env` flags** (command line wins, standard convention):
  - `--env KEY=VALUE` for a key **in** the manifest → overrides its value and
    forces it **active** (suggested→false), keeping the manifest comment. Lets CI
    set a value without editing the file, and lets an operator "turn on" a
    suggestion.
  - `--env KEY=VALUE` for a key **not** in the manifest → appended after all
    manifest entries, sorted, active, no comment (today's behaviour, unchanged).
  - Manifest entries render in **declaration order**; flag-only entries sorted
    after them.
- **Build-side only (no build→runtime contract change) — Option A.** Confirmed
  safe by inspection: `mergeUserEnv` (`cmd/gosd-init/internal/boot/sequence.go:500`)
  applies *any* `gosd.toml [env]` key at runtime, baked or not, so a user who
  uncomments a suggested line gets it applied via the normal card-override path.
  Therefore:
  - `initcfg.Config.Env` stays `map[string]string` and holds **only active**
    effective entries (manifest-active ∪ `--env`); **suggested entries are never
    baked**, so they can't accidentally become runtime defaults — correct by
    construction.
  - Only the **build** render path learns about comments/suggestions.

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

- [ ] `EnvEntry` type + `Render([]EnvEntry, …)` signature change + multi-line
      comment / quoted-value rendering; `EnvEntriesFromMap` helper.
- [ ] Update provsnapshot's two `Render` callers via `EnvEntriesFromMap`.
- [ ] Env-manifest TOML parser + strict decode + validation with actionable
      errors.
- [ ] `--env-file` flag, manifest↔`--env` merge, active-only baked map, `--env`
      help cross-reference.
- [ ] Pipeline passes merged `[]EnvEntry` to `Render`.
- [ ] Unit tests (parse, render golden, merge, baked-defaults) + integration
      test.
- [ ] Docs: `--env-file` section with the worked example + quoting note +
      reflash-self-heal limitation.
- [ ] Quality gates green; PR with bean status/todos updated.
