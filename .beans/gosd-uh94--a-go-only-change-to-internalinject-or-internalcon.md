---
# gosd-uh94
title: A Go-only change to internal/inject or internal/configtree has no structural reason to run the js/ cross-implementation test that would catch it breaking
status: completed
type: bug
priority: normal
created_at: 2026-08-16T04:43:32Z
updated_at: 2026-08-20T05:50:19Z
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

- [x] Decide cheap-fix-now vs. CI-wiring, and record the choice: cheap-fix-now (doc-only). Verified against the actual repo state first (see Summary of Changes) — CI wiring turned out to be unnecessary.
- [x] N/A — CI wiring not needed. `ci.yml`'s `js` job has NO path filter\n      at all (its `on:` block is plain `pull_request:`/`push: branches: [main]`,\n      no `paths:` key, and the job carries no `if:`), so it already runs on\n      *every* PR unconditionally, Go-only changes included — confirmed against\n      PR #198 ("container: preflight-detect remote/SSH docker contexts", diff\n      is internal/container + docs only), whose `js (node 22)`/`js (node 24)`\n      checks both ran and passed. It is also a required status check\n      (`gh api repos/jphastings/gosd/branches/main/protection` lists\n      `js (node 24)` in `required_status_checks.contexts`). Adding a path\n      filter would be a regression (narrowing an unconditional, always-on gate\n      to a conditional one), not a fix.
- [x] Doc-only: updated CLAUDE.md's quality-gates section with the explicit\n      package list (`internal/inject`, `internal/configtree`,\n      `internal/image`), and noted that this is a local-workflow convenience\n      layered on top of CI's already-unconditional `js` gate, not a\n      replacement for a missing structural check.

## Summary of Changes

**The bean's premise, as filed, does not match the repo's actual CI
configuration.** `.github/workflows/ci.yml`'s `js` job has no `paths:`
filter on the workflow's `on:` trigger and no job-level `if:` — it runs
unconditionally on every `pull_request`/push to `main`, regardless of what
changed, and `pnpm run test:integration` (the cross-implementation test this
bean is about) is one of its steps. It is also listed in
`required_status_checks.contexts` on the `main` branch, i.e. it already
blocks merge. Confirmed empirically: PR #198, whose diff is
`internal/container` + docs only, still ran and passed both `js (node 22)`
and `js (node 24)`.

So a Go-only change to `internal/inject`/`internal/configtree`/`internal/image`
already cannot merge without the JS integration suite running and passing —
the CI-layer gap this bean describes does not exist today. Per CLAUDE.md's own
instruction to say so rather than silently diverge when a filed diagnosis
doesn't hold up, this bean is closed on the cheap-fix option only: CLAUDE.md's
quality-gates section now names `internal/inject`, `internal/configtree` and
`internal/image` explicitly alongside `js/` as packages that warrant running
the js gate *locally* before pushing (catching a break before CI does, not
instead of it), and explicitly notes that CI's `js` job is already
unconditional/required so this is a convenience, not a backstop being newly
added. No workflow file was changed — adding a path filter would have made the
gate conditional, i.e. strictly weaker than what already exists.
