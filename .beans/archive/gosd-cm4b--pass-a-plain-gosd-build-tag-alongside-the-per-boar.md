---
# gosd-cm4b
title: Pass a plain `gosd` build tag alongside the per-board tag
status: completed
type: feature
priority: normal
created_at: 2026-08-10T07:53:37Z
updated_at: 2026-08-10T08:00:43Z
---

## Context

`gosd build` (and `gosd run`) compile the user's app with exactly one build
tag today: `boards.BuildTag(b)`, i.e. `gosd_<board-id>` (see
`docs/board-build-tags.md`, bean gosd-1937). That lets an app gate source per
*board*, but there is no way to gate source on "this is being compiled by
gosd at all" — the case where an app wants one behaviour under `go build
./...`/`go test ./...` (host, CI, editor) and another when it is destined for
an SD card image, without maintaining a separate main package.

Enumerating every board tag negatively (`//go:build !gosd_pi_zero_2w &&
!gosd_pi_zero_w && ...`) is the only workaround, and it silently breaks every
time a new board is added.

## Decision (locked)

Every app compile gosd performs gets **two** tags: the bare `gosd` tag, and
the existing per-board `gosd_<board-id>`. Passed as one comma-separated
`-tags` value, i.e. `-tags gosd,gosd_pi_zero_2w`.

- `boards.BuildTag` becomes `boards.BuildTags` (plural) and returns the
  comma-separated pair — one source of truth, so `gosd build` and `gosd run`
  cannot drift.
- The bare tag applies to the **app compile only**, exactly like the board
  tag: gosd-init stays untagged (its argv is load-bearing for
  `initcfg.ComputeIdentity` — see `crossCompileOpts`'s docstring), and
  tsfunnel keeps its own opts.
- `//go:build gosd` therefore means "compiled by gosd for a real image", and
  `//go:build !gosd` is a stable, board-count-independent fallback — which is
  the point.

## Todo

- [x] `boards.BuildTags` returns `gosd,gosd_<id>`; update both call sites
- [x] Unit test: both tags present, board tag still correct per board
- [x] End-to-end: extend `testdata/boardtagfixture` so the integration test
      proves a `//go:build gosd`-gated file reaches /app, and a `!gosd` one
      does not
- [x] `docs/board-build-tags.md`: document the bare tag and the
      `!gosd` fallback pattern
- [x] CLAUDE.md "Naming surfaces" locked decision mentions the bare tag
- [x] Quality gates green, PR open

## Summary of Changes

- `internal/boards`: `BuildTag` → `BuildTags`, now returning
  `"gosd,gosd_<id>"`. Single source of truth for both call sites
  (`cmd/gosd/archbuild.go`'s per-board app compile and `cmd/gosd/run.go`),
  so `gosd build` and `gosd run` can't drift. Nothing else changed about
  tagging: gosd-init and the tsfunnel shim are untouched.
- `cmd/gosd/testdata/boardtagfixture`: added a `//go:build gosd` /
  `//go:build !gosd` pair, each `init()`-printing its own marker string, so
  which marker lands in the built binary's rodata proves whether the bare
  tag was set. Deliberately separate files from the per-board `main()`
  variants, so the two tags are asserted independently.
- Tests: the end-to-end
  `TestBuildAppliesPerBoardBuildTags` → `TestBuildAppliesGosdBuildTags` now
  also asserts each board's `/app` contains the `gosd-tag-set` marker and
  *not* the `gosd-tag-unset` one; `TestCompileForBoardsTagsEveryAppCompile
  WithTheBareGosdTag` covers the same at the seam across three boards
  spanning both arches; `TestBuildTags` updated for the new return value.
  Mutation-checked: reverting `BuildTags` to the old single-tag return makes
  the integration test fail on all four new assertions, not pass vacuously.
- Docs: `docs/board-build-tags.md` retitled and restructured around both
  tags, leading with `gosd`/`!gosd` and demoting the negated-board-list
  fallback to "needs editing whenever you add a board — prefer `!gosd`";
  README quickstart and `docs/runtime.md`'s build-constraints bullet updated
  to match. CLAUDE.md's "Naming surfaces" locked decision now names both
  tags and `boards.BuildTags`.

### Worth knowing

The bare tag is a broader namespace claim than `gosd_<id>` was: a dependency
of a user's app that already uses `//go:build gosd` for its own purposes
would now see it set under `gosd build`. No such collision is known, and
`gosd` is the obvious name for the thing, so this is recorded rather than
designed around.

Gates run in the worktree: `go test ./...`, `go vet ./...`, `gofmt -l .`,
`golangci-lint run ./...` and `GOOS=linux golangci-lint run ./...`. `js/`
untouched.
