---
# gosd-b3m4
title: Let the registry enumerate internal boards, and drop the duplicated kernelspec parity check
status: completed
type: task
priority: normal
created_at: 2026-08-17T20:10:47Z
updated_at: 2026-08-17T21:08:21Z
parent: gosd-8pgg
---

Follow-up to epic gosd-8pgg, cleaning up two things JP flagged when reviewing
its PRs (#311-#318, all merged). No behaviour change; tests and one exported
addition only.

## 1. `boards.AllIncludingInternal()`

`internal/boardset` currently wraps every registration in package-local
`register`/`registerInternal` helpers that also append to a private
`registered` slice, purely so `Registered()` can report internal-only boards —
`boards.All()`/`IDs()` filter them out by design and the registry exposes no
way to ask for them. That was a deliberate deviation (recorded in gosd-ihdn),
and JP's preferred resolution is the one the registry should have had:

- Add `boards.AllIncludingInternal() []Board` to `internal/boards` — every
  registered board sorted by `Name()`, internal-only included. Document how it
  differs from `All()` and when to reach for it.
- `internal/boardset` then calls `boards.Register`/`boards.RegisterInternal`
  **directly**, restoring the verbatim form gosd-ihdn's bean originally asked
  for, and deletes `register`, `registerInternal` and the `registered` slice.
- **Keep `boardset.Registered()`**, delegating to `boards.AllIncludingInternal()`.
  It is a thin wrapper but it carries a real guarantee: calling
  `boards.AllIncludingInternal()` without importing `boardset` returns an empty
  slice, and every check in `internal/repocheck` would then pass vacuously.
  `Registered()` cannot return empty by accident. Say that in its doc comment.

## 2. Drop the duplicated registry <-> kernelspec check

**Correcting the recommendation given to JP:** the advice was to drop
`internal/repocheck`'s copy and keep `internal/kernelspec`'s. Reading both
says the opposite — drop the **kernelspec** one:

- `kernelspec_test.go`'s `TestBoardIDsListsExactlyTheKernelBuildingBoards`
  compares `kernelspec.BoardIDs()` against the registry-derived `allBoardIDs`
  and fails with a bare `BoardIDs() = [...], want [...]` dump.
- `repocheck`'s `TestKernelspecCoversTheRegisteredFleet` covers **both** of the
  same directions and names the file and the exact edit in each message.
- Its forward direction is *also* already covered by `TestSpecResolutionIsComplete`,
  which fatals on a registered board with no spec.

So deleting `TestBoardIDsListsExactlyTheKernelBuildingBoards` loses no
coverage and keeps the better diagnostics.

**Do not delete `allBoardIDs`/`boardIDsFromRegistry`** — `TestSpecResolutionIsComplete`
and the other kernelspec tests still use them.

## Todo

- [x] Add `boards.AllIncludingInternal()` with a doc comment distinguishing it from `All()`
- [x] Cover it in `internal/boards/boards_test.go` (fakeBoard-based, like the `All`/`IDs` cases): a `RegisterInternal` board appears here and not in `All()`
- [x] Simplify `internal/boardset` to direct `boards.Register`/`RegisterInternal` calls, preserving every per-board activation comment verbatim
- [x] Point `Registered()` at `AllIncludingInternal()` and document why the wrapper stays
- [x] Delete `TestBoardIDsListsExactlyTheKernelBuildingBoards` from `internal/kernelspec/kernelspec_test.go`, keeping `allBoardIDs`
- [x] Confirm `internal/repocheck` still passes and still fails usefully when a board is unregistered
- [x] Quality gates (go test / go vet / gofmt / golangci-lint x2)

## Notes

No changeset — `internal/` only, nothing user-facing; use the `no release
notes` label. `boards.AllIncludingInternal` is not public API (`internal/`),
so it carries no semver weight.

## Summary of Changes

- Added `boards.AllIncludingInternal() []Board` to `internal/boards/boards.go`:
  every registered board, public and internal-only, sorted by `Name()`, with a
  doc comment distinguishing it from `All()` and warning that it only sees
  boards some package has actually registered.
- Simplified `internal/boardset/boardset.go`'s `init()` to call
  `boards.Register`/`boards.RegisterInternal` directly, deleting the
  `register`/`registerInternal` helpers and the private `registered` slice.
  Every per-board activation comment (which artifacts release, which bean)
  is preserved verbatim.
- `boardset.Registered()` now delegates to `boards.AllIncludingInternal()`;
  its doc comment explains why the wrapper stays — calling
  `boards.AllIncludingInternal()` without importing `boardset` returns an
  empty slice, which would make every `internal/repocheck` check pass
  vacuously.
- Added `TestRegisterInternalAppearsInAllIncludingInternalButNotAll` to
  `internal/boards/boards_test.go` (fakeBoard-based, matching the existing
  `All`/`IDs` test style).
- Deleted `TestBoardIDsListsExactlyTheKernelBuildingBoards` from
  `internal/kernelspec/kernelspec_test.go` (its coverage is fully carried by
  `internal/repocheck`'s `TestKernelspecCoversTheRegisteredFleet`, which
  names the file and the exact edit in its failure messages). Kept
  `allBoardIDs`/`boardIDsFromRegistry`, still used by
  `TestSpecResolutionIsComplete` and other tests in that file; removed the
  now-unused `sort` import.
- Verified `internal/repocheck` still fails usefully: temporarily commented
  out `boards.Register(cubiea5e.New())` in `boardset` (keeping the import
  live via a blank assignment to avoid a vet failure) and confirmed
  `TestKernelspecCoversTheRegisteredFleet` (and the other fleet-parity
  checks) failed, naming `cubie-a5e` and `internal/boardset/boardset.go`;
  reverted afterward.
- No behaviour change; no contradictions with the bean found.
