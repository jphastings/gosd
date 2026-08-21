---
# gosd-9z43
title: kernelspec.go still calls rock-4se "scaffolding-only," blocked on two beans that completed months ago
status: completed
type: bug
priority: low
created_at: 2026-08-16T04:43:32Z
updated_at: 2026-08-20T05:32:00Z
---

**Severity: Low.** Cosmetic — no code depends on this comment — but it's the
kind of stale claim that erodes trust in every other comment in a package
this dense with load-bearing prose.

## Verified

`internal/kernelspec/kernelspec.go:459-464`, directly above the `"rock-4se"`
entry in the kernel-spec map:

```go
// "rock-4se" is scaffolding-only as of bean gosd-iosp: this board isn't
// registered in internal/boards yet (bean gosd-0vvh), so
// TestBoardIDsListsExactlyTheFiveKernelBuildingBoards in
// kernelspec_test.go fails until that board profile lands and the test
// is updated to include it - a known, reported cross-bean coupling, not
// silently worked around here. See the bean body's "Scaffolding status"
// note.
```

Both blocking conditions are already resolved:

- `gosd-0vvh` ("rock-4se board profile: extlinux bootloader, raw writes...")
  — **status: completed**, archived.
- `gosd-iosp` ("ROCK 4SE: trimmed mainline kernel build") — **status:
  completed**, archived.
- `rock-4se` **is** registered: `cmd/gosd/build.go:52`,
  `boards.Register(rock4se.New())`, with a comment there explicitly saying
  "rock-4se is public."
- The test the comment names doesn't even exist under that name anymore —
  it's `TestBoardIDsListsExactlyTheKernelBuildingBoards` now
  (`internal/kernelspec/kernelspec_test.go:24`, "Five" dropped), confirming
  the board count moved on without this comment being revisited.

## Fix

Delete the stale block, or replace it with a one-line note (if any residual
context is worth keeping) that rock-4se shipped as a public board in
`gosd-0vvh`/`gosd-iosp`.

## Todos

- [x] Remove or rewrite `kernelspec.go:459-464`
- [x] Skim the rest of `kernelspec.go` for any other board's entry still
      carrying a "scaffolding-only" / "not yet registered" comment from its
      own bring-up era (none found)

## Summary of Changes

Deleted the stale "rock-4se is scaffolding-only" comment block above the
`"rock-4se"` kernelspec entry: both blocking beans (gosd-0vvh, gosd-iosp)
completed months ago, rock-4se is registered and public
(`boards.Register(rock4se.New())` in `cmd/gosd/build.go`), and the named
test (`TestBoardIDsListsExactlyTheFiveKernelBuildingBoards`) doesn't exist
under that name any more. Skimmed the rest of `kernelspec.go` for similar
stale scaffolding/not-yet-registered comments on other boards' entries —
found none.
