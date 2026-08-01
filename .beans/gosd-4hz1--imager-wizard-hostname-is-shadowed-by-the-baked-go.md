---
# gosd-4hz1
title: Imager wizard hostname is shadowed by the baked gosd.toml hostname on every image
status: completed
type: bug
priority: high
created_at: 2026-07-31T20:14:57Z
updated_at: 2026-07-31T20:14:57Z
---

Found during gosd-ry3b's implementation (PR #161), recorded in that bean;
filed separately here.

`gosd build` always renders `hostname = "..."` into the card's gosd.toml
(the --hostname flag defaults to the sanitized main-package name), and
the locked provisioning precedence is gosd.toml > cloud-init > baked
config.json. Consequence: the hostname an end user types into Imager's
customization wizard (cloud-init user-data) is ALWAYS outranked by the
baked default — the wizard's headline feature silently doesn't take
effect. Wizard WiFi is unaffected (gosd.toml's [wifi] block ships
commented out unless baked).

**Fix direction (decide in this bean):** either stop rendering an
uncommented hostname line into gosd.toml at build time (ship it
commented, like [wifi], so the precedence chain falls through to
cloud-init), or make the renderer distinguish "baked default" from
"operator-set" hostname. The first is simpler and matches the [wifi]
pattern; check interaction with gosd-ry3b's snapshot classification
(a freshly flashed card's gosd.toml equalling the rendered template is
how hand-edits are detected — shipping the hostname commented actually
sharpens that test). Behavioral test: wizard-provisioned hostname takes
effect on a stock image; hand-edited gosd.toml hostname still wins.

## Decision

Went with the renderer-distinguishes-intent option, which subsumes the
simpler "always comment it" direction: `gosdtoml.Render` now takes a
`bakeHostname bool` alongside `hostname`. A commented render always shows
`hostname`'s value as the example (not a generic placeholder), so the card
still documents its effective default even while the line stays inert.

`boards.BuildConfig` gained `HostnameExplicit bool`. `gosd build`/`gosd run`
set it from `hostname != ""` (the same "was --hostname actually passed"
check the existing default-substitution already relied on), before
substituting in the sanitized-package-name default. The build pipeline
passes it straight through to `gosdtoml.Render`.

**Explicit vs default, decided:** an explicit `--hostname` bakes uncommented
into gosd.toml and always wins (matching a hand-edit — the developer chose
it deliberately, same as today). The sanitized-default hostname (no
`--hostname` passed) renders commented out, like `[wifi]`, so the locked
gosd.toml > cloud-init > config.json precedence falls through to an Imager
wizard's cloud-init hostname on the common case. `config.json`'s hostname
is unaffected either way — it still always carries the resolved
`Hostname` as gosd-init's last-resort fallback.

## Summary of Changes

- `internal/gosdtoml/template.go`: `Render` takes a new `bakeHostname bool`
  parameter. The hostname line renders uncommented only when
  `hostname != "" && bakeHostname`; otherwise it renders commented, showing
  `hostname`'s value (falling back to the generic "my-device" placeholder
  when it's also empty) as the example.
- `internal/boards/boards.go`: `BuildConfig` gained `HostnameExplicit bool`,
  documented as distinguishing an operator's deliberate `--hostname` from
  the sanitized-package-name default.
- `internal/pipeline/pipeline.go`: `Assemble` passes
  `opts.Config.HostnameExplicit` through to `gosdtoml.Render`.
- `cmd/gosd/build.go`, `cmd/gosd/run.go`: compute `hostnameExplicit :=
  hostname != ""` before substituting the default, thread it into
  `boards.BuildConfig`, and note the commented-vs-baked behavior in the
  `--hostname` help text.
- `cmd/gosd-init/internal/provsnapshot/provsnapshot.go`: both internal
  `gosdtoml.Render` call sites (the reflash write-back in `heal`, and the
  snapshot-directory bookkeeping copy in `encode`) now pass
  `bakeHostname=true` — a restored value is provable operator intent, and
  the snapshot's own gosd.toml copy (under `/data`, never seen by a user)
  must round-trip whatever `Effective.Hostname` actually is. Updated the
  package doc and `effective`'s docstring, which previously asserted "gosd
  build always renders a hostname into gosd.toml" — no longer true for the
  default case, though `freshHostname`'s emptiness check already handled a
  commented-out line identically to a template match, so no runtime-logic
  change was needed there.
- Tests: `internal/gosdtoml/template_test.go` covers the new parameter,
  including a dedicated case for a non-empty-but-unbaked hostname (shown
  commented, Parses as empty). `internal/pipeline/pipeline_test.go` gained
  `TestAssembleWritesCommentedGosdTomlHostnameForNonExplicitDefault`,
  reading gosd.toml back from a built image to confirm a non-explicit
  hostname is commented out in gosd.toml while still landing in
  config.json. `cmd/gosd-init/internal/provsnapshot/provsnapshot_test.go`'s
  `TestReflashRestoresHostnameAndWifiOnlyWhenTheFreshBootHasNone` was
  updated to model a real default build's gosd.toml (empty hostname, not
  equal to the baked default) and now asserts a wizard hostname is
  correctly snapshotted as effective provisioning once it's no longer
  shadowed.

Companion bean gosd-nchn (`--hostname` isn't sanitized/validated at parse
time) is unaddressed here, as scoped.
