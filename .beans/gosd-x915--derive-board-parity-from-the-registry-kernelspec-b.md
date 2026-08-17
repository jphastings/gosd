---
# gosd-x915
title: 'Derive board parity from the registry: kernelspec, build/boards, fake-artifacts, COMPATIBILITY.md'
status: in-progress
type: task
priority: normal
created_at: 2026-08-17T16:55:53Z
updated_at: 2026-08-17T17:52:27Z
parent: gosd-8pgg
blocked_by:
    - gosd-ihdn
---

Part of epic gosd-8pgg. Stacked on gosd-ihdn (needs `internal/boardset` and
`internal/repocheck`).

Four separate CLAUDE.md rules say "when you add/activate a board, remember to
also update X". Each is a manual step whose omission is caught late or not at
all — PR #205 lost time to exactly this, where a warm artifact cache in the real
`$HOME` masked a missing fixture locally and only CI noticed. Replace all four
with assertions derived from the registry.

## Locked decisions

- Everything lives in `internal/repocheck/boards_test.go`. **No hand-maintained
  board lists** — every expectation derives from `boardset.Registered()` or
  `boards.All()`.
- Assertions, each in **both directions** (missing *and* orphaned):

  | Check | Scope |
  |---|---|
  | `kernelspec.Get(id)` returns ok | all boards |
  | `build/boards/<id>/` is a directory | all boards |
  | every `ArtifactRef.Name` exists and is non-empty in `cmd/gosd/testdata/fake-artifacts/` | `boards.All()` |
  | `DisplayName()` has a row in COMPATIBILITY.md's `## Bring-up status` table | `boards.All()` |
  | `len(boards.All()) > 0` | — |

- **A URL-bearing `ArtifactRef` still needs a fixture.** `ResolveArtifacts` in
  `internal/boards/artifacts.go` checks `--artifacts-dir` first and falls back
  to a real network fetch, which a warm `$HOME` cache can satisfy locally while
  only CI catches it. No URL exemption. (Confirmed by the tree: the Pi firmware
  blobs are all URL-bearing and all present as fixtures.)
- **The qemu-virt exemption is `boards.All()` itself** — no exemption map.
  Iterating it means flipping `RegisterInternal`→`Register` *automatically*
  starts demanding fixtures and a COMPATIBILITY.md row. That is CLAUDE.md's
  activation rule made mechanical rather than restated.
- The fake-artifacts reverse check needs a **derived** non-board set, not a
  literal one: `cacerts.ArtifactName` plus the names in
  `cloudflaredpin.ByGOARCH`. Adding a third feature artifact must not require
  editing this test.
- Scope the `build/boards/` reverse scan to `build/boards/*` only —
  `build/artifacts/VERSION` lives next door and is knope's.
- **Do not assert anything about COMPATIBILITY.md's second (board x feature)
  table.** Its column headers are deliberately abbreviated ("Pi Zero 2W" for
  `Raspberry Pi Zero 2W`) in 5 of 7 columns. Say so in a comment so nobody
  "fixes" it later.
- `len(boards.All()) > 0` exists to make a dropped blank import in `cmd/gosd`
  fail with a message that names the cause.
- Failure messages must name the exact file and the exact edit to make — they
  are the actual interface of this test.

## Todo

- [x] `internal/repocheck/boards_test.go` with the five checks, both directions
- [x] Verify each bites: comment out a `boards.Register` line and confirm the message names the board and the missing artefact
- [x] Amend CLAUDE.md's "Activating a board (internal -> public) must also add its artifacts to `cmd/gosd/testdata/fake-artifacts/`" — reduce to a pointer at the test now that it is enforced
- [x] Amend CLAUDE.md's "Board or feature status changes must update COMPATIBILITY.md in the same PR" — note the bring-up table is enforced, the feature table is not
- [x] Quality gates (go test / go vet / gofmt / golangci-lint x2)

## Notes

No changeset — internal only, use the `no release notes` label.

## Summary of Changes

`internal/repocheck/boards_test.go` holds five checks, each derived from the
registry and each in both directions:

- `TestBoardRegistryIsPopulated` — an empty registry passes every other check
  vacuously, so it fails here first, distinguishing "nothing registered" from
  "everything registered internal-only".
- `TestKernelspecCoversTheRegisteredFleet` — `kernelspec.Get(id)` for every
  `boardset.Registered()` board, and `kernelspec.BoardIDs()` back the other
  way.
- `TestBuildBoardsDirsCoverTheRegisteredFleet` — `build/boards/<id>/` per
  board, and a scan of `build/boards/*` back (scoped there, so knope's
  `build/artifacts/VERSION` is untouched).
- `TestFakeArtifactsCoverEveryPublicBoard` — every `boards.All()` board's
  `ArtifactRef.Name` present and non-empty in
  `cmd/gosd/testdata/fake-artifacts/`, no URL exemption; the reverse set is
  derived from `cacerts.ArtifactName` plus `cloudflaredpin.ByGOARCH`, so a
  third feature artifact needs no edit here.
- `TestCompatibilityBringUpRows` — a `DisplayName()` row per `boards.All()`
  board in COMPATIBILITY.md's `## Bring-up status` table, and back. The
  board x feature table is deliberately not asserted (5 of its 7 headers are
  abbreviated); a comment says so.

All five pass on the tree as it stands — no drift was hiding. Each was proven
to bite by mutation: unregistering rock-4se raises four orphan failures naming
the board and the exact file to edit; flipping qemu-virt to `register` demands
a COMPATIBILITY.md row on its own; a renamed kernelspec key, an extra
`build/boards/` entry, and a deleted or truncated fixture each name the file
and the edit.

Worth recording: `build/boards/<id>` is also a Go package the board imports,
so for today's boards its absence is already a compile error — the check earns
its place on the orphan direction and on a future board that ships no
manifest package.

CLAUDE.md's fake-artifacts activation rule and its COMPATIBILITY.md rule are
now pointers at the check, the latter noting the bring-up table is enforced
and the feature table is not.
