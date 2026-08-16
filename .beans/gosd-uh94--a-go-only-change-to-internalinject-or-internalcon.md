---
# gosd-uh94
title: 'A Go-only change to internal/inject or internal/configtree has no structural reason to run the js/ cross-implementation test that would catch it breaking'
status: todo
type: bug
priority: normal
created_at: 2026-08-16T04:43:32Z
updated_at: 2026-08-16T04:43:32Z
---

**Severity: Medium.** The safety net that exists is good; the gap is that
nothing forces a change on one side of the boundary to trip it.

## Verified — the coupling, and how it's guarded

The build-time injection manifest and the TypeScript downloader
(`js/packages/gosd`) share a contract implemented independently in two
languages:

- `internal/inject.Manifest`'s JSON shape (`gosd_inject`, `board`, `image`,
  `placeholders`, `config`, the `manifestSchemaVersion` literal `1`) is
  mirrored field-for-field by `js/packages/gosd/src/downloads/manifest.ts`.
- Byte-padding (`internal/configtree`'s `pad`, newline-padding to a reserved
  size) is independently reimplemented as `content.ts`'s `padTo`.
- The env-name/`GOSD_*` reservation rule is validated twice: Go's
  `configtree.checkEnvValue` and TypeScript's `content.ts checkEnvPath`.

The one thing that actually catches drift between these is
`internal/cmd/injectfixture` (a Go fixture generator, deliberately kept as
normal Go module code per CLAUDE.md so it runs under the standard Go gates)
feeding `js/packages/gosd/integration/inject.integration.test.ts`, a
cross-implementation integration test. That test only runs as part of the
`js/` quality gate: `cd js && pnpm install --frozen-lockfile && ... && pnpm
run test:integration`.

CLAUDE.md's own quality-gates section already tells a contributor to run
that gate "when a change touches `js/`" — which is the right instruction for
someone editing `js/` files, but gives no signal at all to someone editing
`internal/inject`, `internal/configtree`, or `internal/image`'s
`ReportRanges` byte-range logic, none of which touch a single file under
`js/`. A change to any of those that breaks the shared contract (a padding
byte, an off-by-one range, a schema field) would pass every Go gate cleanly
and only surface once someone happens to also run the JS integration suite —
or, worse, once an end user's download hits
`GosdPlaceholderNotPristineError`/`GosdImageHashMismatchError` in production.

## Fix direction (not locked)

- Cheapest: point CLAUDE.md's "run the js gate when a change touches `js/`"
  instruction at the actual dependency, not the directory — name
  `internal/inject`, `internal/configtree`, and `internal/image` explicitly
  as also requiring it.
- Better: wire CI so a PR touching any of those three Go packages
  automatically runs the `js/` integration test job, the same way
  `change-file-check.yml` already inspects a PR's diff for other
  path-based gating. This makes the safety net structural instead of relying
  on a contributor remembering the rule.

## Todos

- [ ] Decide cheap-fix-now vs. CI-wiring, and record the choice
- [ ] If CI wiring: add a path filter to the relevant workflow so
      `internal/inject/**`, `internal/configtree/**`, and `internal/image/**`
      changes trigger the `js` job even when no `js/` file changed
- [ ] If doc-only for now: update CLAUDE.md's quality-gates section with the
      explicit package list
