---
# gosd-ihdn
title: Move board registration into internal/boardset so tests can populate the registry
status: in-progress
type: task
priority: normal
created_at: 2026-08-17T16:55:33Z
updated_at: 2026-08-17T17:22:11Z
parent: gosd-8pgg
---

Structural prerequisite for the rest of epic gosd-8pgg. Pure refactor plus one
empty package; no behaviour change.

Boards are registered in `cmd/gosd/build.go`'s `init()`, so `boards.All()` and
`boards.IDs()` are only ever populated inside `package main`. That is why
`internal/kernelspec/kernelspec_test.go` hand-maintains **two** duplicate board
lists — the exact drift this epic exists to close. Nothing outside `cmd/gosd`
can ask the registry what boards exist.

## Locked decisions

- **New package `internal/boardset`.** Its `init()` takes the body of
  `cmd/gosd/build.go`'s current `init()` **verbatim, comments included** —
  build.go:36-63 carries per-board activation rationale (which artifacts release
  made each board public, which bean flipped it) that must not be lost.
- It exports exactly one symbol:
  `func Registered() []boards.Board` — every board it registers, public *and*
  internal-only, sorted by `Name()`. `boards.All()`/`IDs()` deliberately omit
  internal-only boards; parity checks covering the whole fleet need the full
  set and re-derive the public subset with `boards.IsInternal`.
- `cmd/gosd/build.go` loses its `init()` and its eight board sub-package
  imports, gaining a blank import of `boardset` **with a comment** saying that
  dropping it silently builds a gosd that knows about zero boards.
- **Also create `internal/repocheck/doc.go`** — package doc only, no symbols.
  It is the home for repo-wide invariants no production package owns, and
  landing it here (rather than in each sibling PR) keeps the four checks that
  follow independent of each other. Say in the doc comment what belongs there
  and that its tests read the repo root at `"../.."`.
- Import safety is **already verified**: `go list -deps ./internal/boards/...`
  does not contain `internal/kernelspec` and vice versa — they are siblings
  sharing `build/boards/*` leaves. No cycle is introduced.

## Todo

- [x] Create `internal/boardset/boardset.go` with the moved `init()` and `Registered()`
- [x] Strip `init()` + board imports from `cmd/gosd/build.go`, add the commented blank import
- [x] Delete `allBoardIDs` (kernelspec_test.go:22) and derive from `boardset.Registered()`
- [x] Delete `boardsByID` (kernelspec_test.go:99) and derive via `boards.Find`; remove the now-unreachable `t.Fatalf("no internal/boards.Board wired up in this test for %q")` branch and the 8 board sub-package imports
- [x] Derive the hardcoded image-name list at `cmd/gosd/build_integration_test.go:1129` from `boards.All()`
- [x] Create `internal/repocheck/doc.go`
- [x] Amend CLAUDE.md: the "Adding a `kernelspec` entry also means updating the board-enumerating test lists" sentence now over-states the work — the board-count list and the kernelspec-outputs-vs-Artifacts map are derived; only the Rockchip DTS-patch allowlist remains hand-maintained
- [x] Quality gates (go test / go vet / gofmt / golangci-lint x2)

## Notes

`boards.register()` panics on duplicate registration, so a half-finished move
fails loudly on the first test rather than silently double-registering.

No changeset — internal only, use the `no release notes` label.

## Summary of Changes

**`internal/boardset` (new).** Its `init()` carries the eight registration
calls from `cmd/gosd/build.go`, per-board activation comments verbatim. One
deviation from the locked wording: the calls go through package-local
`register`/`registerInternal` one-liners rather than `boards.Register`/
`boards.RegisterInternal` directly, because `Registered()` has to report
internal-only boards and `internal/boards` exposes no way to enumerate them
(`All()`/`IDs()` filter them out by design). The alternative was a second
hand-maintained list inside the very package whose job is to end them.
`Registered()` returns a sorted copy of what the package registered, public
and internal-only alike.

**`cmd/gosd/build.go`** loses its `init()` and the eight board sub-package
imports, gaining `_ "github.com/jphastings/gosd/internal/boardset"` with a
comment recording that dropping it still COMPILES — it just produces a gosd
that knows about zero boards. `internal/boards`' package doc and
`Register`'s docstring now point at `internal/boardset` instead of
`cmd/gosd`.

**`internal/kernelspec/kernelspec_test.go`.** `allBoardIDs` is now derived
from `boardset.Registered()`, and `boardsByID` is gone —
`TestKernelSpecOutputsMatchBoardArtifacts` resolves each board with
`boards.Find`, whose `ok` is discarded because the ids came from the registry
in the first place (the "no internal/boards.Board wired up in this test"
branch was describing wiring that no longer exists). Eight board sub-package
imports removed.

**`cmd/gosd/build_integration_test.go`.**
`TestBuildWithNoBoardFlagBuildsAllBoards` derives both the expected image
names and the expected count from `boards.All()`, so the hardcoded seven-name
list and the literal `!= 7` are gone. The internal-only exclusion check
(no `hello-qemu-virt.img`) is unchanged and still explicit.

**`internal/repocheck/doc.go` (new).** Package doc only, no symbols: says
what belongs there, that its content is `_test.go` files reading the repo
root at `"../.."`, and that a check needing the fleet uses `boardset`
rather than a list of its own.

**CLAUDE.md.** The `gosd build-kernel` bullet's registration site is now
`internal/boardset`, and its closing sentence no longer asks for three test
lists to be updated: the board-count list and the outputs-vs-`Artifacts()`
map are derived from the registry, leaving only the Rockchip DTS-patch
allowlist hand-maintained (a board either should or should not carry patches,
which only a human knows).

### Verification

All gates green: `go test ./...`, `go vet ./...`, `gofmt -l .`,
`golangci-lint run ./...` and `GOOS=linux golangci-lint run ./...`.
`gosd build --help` still lists all seven public boards, so the blank import
does populate the registry. One gotcha worth recording: `go test ./...` under
a cold isolated `GOCACHE` blows `cmd/gosd`'s default 10-minute timeout
(`internal/build` alone spends ~230s cross-compiling); it passes in ~250s
once the cache is warm, and the timeout is not a symptom of this change.

Nothing contradicted the bean's assumptions. The import-safety claim holds —
`go list -deps ./internal/boards/...` still contains no `kernelspec`, and
`internal/kernelspec`'s deps are `build/boards/*` leaves only, so
`kernelspec_test` importing `boardset` introduces no cycle.
