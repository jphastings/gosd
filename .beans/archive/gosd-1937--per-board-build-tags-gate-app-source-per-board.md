---
# gosd-1937
title: 'Per-board build tags: gate app source per board'
status: completed
type: feature
priority: normal
created_at: 2026-07-09T21:47:16Z
updated_at: 2026-07-10T00:05:28Z
parent: gosd-vi0n
---

Let a developer keep board-specific source in their app and have `gosd build`
compile the right file per board, by passing a per-board Go build tag to the
app compile. Motivating shape: a `stuff_pi-zero-2w.go` next to a
`stuff_nanopi-zero2.go`, each gated to its board with build tags.

## Why real build tags, not filenames (locked)

Go's filename build constraints only honour known GOOS/GOARCH tokens
(`_linux.go`, `_arm64.go`). A board id like `nanopi-zero2` is neither, so
`stuff_nanopi-zero2.go` compiles for **every** board. Making filenames
load-bearing would force gosd to reimplement Go's file selection and would
break plain `go build` / `go test ./...` (all board files compile together →
duplicate symbols) and editor tooling. So gosd passes a real `//go:build` tag;
the `_<board>.go` filename suffix is a cosmetic convention only.

## Locked decisions

- **Tag form:** `gosd_<id>`, board id underscored (hyphens are illegal in build tags): `pi-zero-2w` → `gosd_pi_zero_2w`, `nanopi-zero2` → `gosd_nanopi_zero2`. New public naming surface: build tags matching `gosd_*` (alongside env vars `GOSD_*`).
- **Scope: app compile only.** gosd-init is NOT tagged and keeps its per-arch dedup. Do not thread the tag into `CrossCompileGosdInit`.
- **Consequence:** the app now compiles **once per board** (the tag is unique per board), not once per arch. Accepted cost — Go's build cache keeps the extra arm64 passes cheap (only app packages that reference a `gosd_*` tag recompile; stdlib and untouched deps are cache hits). Do not build a "detect whether the app uses any board tag" optimisation; always pass the tag.

## Work

- [x] `internal/boards/boards.go`: add `func BuildTag(b Board) string` returning `"gosd_" + strings.ReplaceAll(b.Name(), "-", "_")`. The `gosd_` prefix keeps it a valid tag identifier even for ids starting with a digit. Unit test the representative mappings (`pi-zero-2w`, `nanopi-zero2`).
- [x] `internal/build/build.go`: add a `tags string` param to `CrossCompile` (empty = none); insert `-tags <tags>` into the argv before the package path. `archEnv` and `requireMainPackage` unchanged.
- [x] `cmd/gosd/archbuild.go`: replaced `compileForArchs(archs, …)` with board-aware `compileForBoards(selected []boards.Board, …)`. Compiles the app once per board (output `app-<board.Name()>`, tagged with `boards.BuildTag(b)`); compiles gosd-init once per distinct `arch.Key()` (unchanged, no tag). Returns `map[string]archBinaries` keyed by `b.Name()` — boards sharing an arch reuse the same `initPath`, each gets its own `appPath`. `compileApp` seam updated to the new `CrossCompile` signature (`pkgPath, outputPath, tags string, arch`).
- [x] `cmd/gosd/build.go` (`runBuild`): passes `selected` straight to `compileForBoards` (the intermediate `archs` slice is gone); looks up binaries by `b.Name()` instead of `b.Arch().Key()` in the assembly loop.
- [x] Docs: added `docs/board-build-tags.md` (linked from README's quickstart and `docs/runtime.md`'s Build constraints section) — covers the `gosd_<id>` tag per board id, the two fallback patterns (negated constraint / sole-definer), and that the `_<board>.go` filename suffix is cosmetic only.
- [x] Updated the **Naming surfaces** locked-decision line in `CLAUDE.md` to add the `gosd_<board-id>` build-tag surface. Left COMPATIBILITY.md alone: it's a board×feature matrix, and per-board build tags are a board-agnostic developer capability (applies identically to every board), so a row would be misleading (all-✅ says nothing).
- [x] Tests: extended `cmd/gosd/archbuild_test.go` (real board values via `pizero2w`/`radxazero3e`/`nanopizero2`/`pizerow` `New()`) — two boards on one arch compile the app twice with each board's own tag and gosd-init once; a mixed-arch selection adds one more init pass; distinct app paths per board; app-failure still blocks init. Added `internal/build/build_test.go`'s `TestCrossCompilePlacesTagsBeforePackagePath` (fixture package that only compiles under the right tag). Added a full end-to-end integration test, `cmd/gosd/archbuild_boardtag_integration_test.go`, that runs a real `gosd build --board pi-zero-2w --board nanopi-zero2` against a new `cmd/gosd/testdata/boardtagfixture` app and asserts each image's /app carries its own board's marker (not the fallback, not the other board's) while gosd-init stays byte-identical across both — closing the loop on the bean's full acceptance criterion, not just the unit-level seam tests.

## Acceptance

`go test ./...`, `go vet ./...`, `gofmt -l .`, and both `golangci-lint run`
invocations (native + `GOOS=linux`) pass. A fixture app with a fallback
`main.go` (negated constraint) and a `//go:build gosd_pi_zero_2w`-gated file
compiles cleanly under plain `go build ./...` / `go test ./...`, and
`gosd build --board pi-zero-2w --board nanopi-zero2 <fixture>` produces two app
binaries each carrying the board-specific code, while the two arm64 gosd-init
binaries remain byte-identical (init dedup preserved).


## Summary of Changes

- `internal/boards.BuildTag(b Board) string` returns `gosd_<id>` (hyphens
  underscored), the tag passed to the app compile only.
- `internal/build.CrossCompile` gained a `tags string` param, inserting
  `-tags <tags>` before the package path when non-empty; every existing
  caller (build_test.go, cmd/gosd/run.go) updated. `CrossCompileGosdInit`
  is untouched, per the bean's locked scope.
- `cmd/gosd/archbuild.go`'s `compileForArchs([]boards.Arch, ...)` became
  `compileForBoards([]boards.Board, ...)`: the app compiles once per board
  (its own `boards.BuildTag`), gosd-init still compiles once per distinct
  `arch.Key()`. Result keyed by `b.Name()`; boards sharing an arch share one
  `initPath` but never an `appPath`.
- `cmd/gosd/build.go`'s `runBuild` passes `selected` straight to
  `compileForBoards` (the intermediate `archs` slice is gone) and looks up
  binaries by board name.
- `cmd/gosd/run.go` also now passes `boards.BuildTag(b)` to its own
  `CrossCompile` call (for `gosd run`'s qemu-virt build) — not explicitly
  required by the bean's work items, but needed once `CrossCompile`'s
  signature changed, and consistent with the feature: a `//go:build
  gosd_qemu_virt`-gated file should be honored by `gosd run` too.
- Docs: new `docs/board-build-tags.md` (tag table, both fallback patterns,
  the cosmetic-filename-suffix note), linked from README's quickstart and
  from `docs/runtime.md`'s Build constraints section. `CLAUDE.md`'s Naming
  surfaces line now mentions the `gosd_<board-id>` tag surface.
  COMPATIBILITY.md deliberately left alone (see the checked-off work item
  above for why).
- Tests: `internal/boards.BuildTag` unit test; `internal/build`'s
  `TestCrossCompilePlacesTagsBeforePackagePath` (a `testdata/boardtag`
  fixture whose default file only compiles when untagged, and is written to
  fail, so a tagged-build-succeeds/untagged-build-fails pair proves the tag
  reached `go build`); `cmd/gosd/archbuild_test.go` rewritten around real
  board values for the per-board-app/per-arch-init contract; and a new full
  end-to-end test, `cmd/gosd/archbuild_boardtag_integration_test.go`, using
  a new `cmd/gosd/testdata/boardtagfixture` app (fallback + two board-gated
  variants) — a real two-board `gosd build` produces the right marker in
  each image's /app and a byte-identical gosd-init across both.
