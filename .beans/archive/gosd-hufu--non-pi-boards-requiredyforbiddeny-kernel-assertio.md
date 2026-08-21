---
# gosd-hufu
title: Non-Pi boards' RequiredY/ForbiddenY kernel assertions are hand-copied with no cross-check, unlike the Pi fragment-derived list
status: completed
type: task
priority: normal
created_at: 2026-08-16T04:43:32Z
updated_at: 2026-08-20T06:06:48Z
---

**Severity: Medium.** Nothing is wrong today — this is a process gap, not a
bug — but it's the one place in `internal/kernelspec` where a wrong
hand-edit wouldn't be caught until a real board fails to boot on a bench
pass, rather than by `go test ./...`.

## Verified — the asymmetry

For Pi boards, `RequiredY` is *mechanically derived* from the config
fragment (`requiredYFromFragment`, `internal/kernelspec/kernelspec.go:190-204`
— every literal `CONFIG_*=y` line in the fragment, collected automatically),
and `TestPiRequiredYIsDerivedFromFragment` enforces that the two can never
drift apart.

For every Rockchip/Allwinner board, `RequiredY`/`ForbiddenY` are hand-written
literal slices, copy-pasted board to board with comments that say so
outright:

```
kernelspec.go:489:  // See the radxa-zero-3e RequiredY comment above - same origin, now
kernelspec.go:544:  // See the radxa-zero-3e RequiredY comment above - same origin,
kernelspec.go:595:  // See the radxa-zero-3e RequiredY comment above - same origin, now
```

There used to be a second, independent copy of these lists in each board's
shell-script build recipe, which at least gave a human something to diff
against; those scripts (and their `required_y`/`forbidden_y` arrays) were
deleted when `kernelspec`/`kernelbuild` replaced them (bean `gosd-07fl`). The
declarative `KernelSpec` is now the *only* copy — correct per the "one source
of truth" goal of that bean, but it also means a hand-edit that's simply
wrong (a typo'd `CONFIG_` name, a stale assertion left over from a defconfig
bump) has nothing to catch it before a hardware bring-up.

## Fix direction (not locked)

Not proposing the Pi mechanism verbatim — Rockchip/Allwinner boards' required
configs come from DTS patches and board-specific peripheral needs, not one
fragment, so there's no single file to derive from automatically the way
`requiredYFromFragment` does. Options worth weighing:
- A test that at least asserts every `RequiredY` entry for a given board
  actually appears (as `=y`) somewhere in that board's own fragment/patches,
  catching the "typo'd CONFIG name" and "assertion for a config this board
  doesn't even set" classes of error even without full derivation.
- A shared helper that boards opt into for the parts of `RequiredY` that
  *are* fragment-derivable, falling back to a smaller hand-written list for
  the genuinely bespoke assertions (DTS-patch-enabled peripherals).

## Todos

- [x] Decided: option 1 from the fix direction (a test asserting every
      hand-written RequiredY/ForbiddenY entry appears/doesn't appear as a
      literal =y line in the board's own ConfigFragment), not full
      derivation - a Rockchip/Allwinner/qemu-virt fragment deliberately
      restates `make defconfig` baseline symbols alongside GoSD's own
      requirements, so "every =y line is required" (the Pi mechanism) isn't
      true there and would over-assert.
- [x] Applied to `radxa-zero-3e`, `nanopi-zero2`, `rock-4se`, `cubie-a5e`
      (and qemu-virt, which also hand-writes RequiredY/ForbiddenY) via
      `TestNonPiRequiredYForbiddenYAppearInOwnFragment` in
      internal/kernelspec/kernelspec_test.go. All five boards currently
      pass with no drift.
- [x] Noted in `kernelspec.go`'s package doc (new paragraph explaining the
      Pi-derives-fully vs non-Pi-hand-writes-and-cross-checks split, and
      pointing a future board at the right pattern).

## Summary of Changes

Added `TestNonPiRequiredYForbiddenYAppearInOwnFragment` (internal/kernelspec/kernelspec_test.go): for every board whose RequiredY/ForbiddenY is hand-maintained (radxa-zero-3e, nanopi-zero2, rock-4se, cubie-a5e, qemu-virt), every RequiredY entry must appear as a literal `CONFIG_FOO=y` line in that board's own ConfigFragment, and every ForbiddenY entry must not. DTS patches never touch Kconfig, so the fragment is confirmed (by inspection, for every board) to be the sole source of every RequiredY/ForbiddenY symbol - this cross-check catches a typo'd CONFIG_ name or a dead assertion for a config the board's fragment never sets, without over-asserting (unlike the Pi boards, these fragments deliberately restate `make defconfig` baseline symbols, so full derivation would be wrong).

Also extended kernelspec.go's package doc explaining why Pi boards derive RequiredY fully from the fragment while every other board hand-writes-and-cross-checks, so a future board addition knows which pattern to follow.

No drift found: all five boards currently pass.
