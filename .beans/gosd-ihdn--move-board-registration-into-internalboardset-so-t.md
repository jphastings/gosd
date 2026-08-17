---
# gosd-ihdn
title: Move board registration into internal/boardset so tests can populate the registry
status: todo
type: task
created_at: 2026-08-17T16:55:33Z
updated_at: 2026-08-17T16:55:33Z
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

- [ ] Create `internal/boardset/boardset.go` with the moved `init()` and `Registered()`
- [ ] Strip `init()` + board imports from `cmd/gosd/build.go`, add the commented blank import
- [ ] Delete `allBoardIDs` (kernelspec_test.go:22) and derive from `boardset.Registered()`
- [ ] Delete `boardsByID` (kernelspec_test.go:99) and derive via `boards.Find`; remove the now-unreachable `t.Fatalf("no internal/boards.Board wired up in this test for %q")` branch and the 8 board sub-package imports
- [ ] Derive the hardcoded image-name list at `cmd/gosd/build_integration_test.go:1129` from `boards.All()`
- [ ] Create `internal/repocheck/doc.go`
- [ ] Amend CLAUDE.md: the "Adding a `kernelspec` entry also means updating the board-enumerating test lists" sentence now over-states the work — the board-count list and the kernelspec-outputs-vs-Artifacts map are derived; only the Rockchip DTS-patch allowlist remains hand-maintained
- [ ] Quality gates (go test / go vet / gofmt / golangci-lint x2)

## Notes

`boards.register()` panics on duplicate registration, so a half-finished move
fails loudly on the first test rather than silently double-registering.

No changeset — internal only, use the `no release notes` label.
