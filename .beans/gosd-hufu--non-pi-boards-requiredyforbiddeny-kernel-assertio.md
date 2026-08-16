---
# gosd-hufu
title: 'Non-Pi boards'' RequiredY/ForbiddenY kernel assertions are hand-copied with no cross-check, unlike the Pi fragment-derived list'
status: todo
type: task
priority: normal
created_at: 2026-08-16T04:43:32Z
updated_at: 2026-08-16T04:43:32Z
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

- [ ] Decide a verification mechanism (see options above) and record the
      choice in this bean before implementing
- [ ] Apply it to `radxa-zero-3e`, `nanopi-zero2`, `rock-4se`, `cubie-a5e`
- [ ] Note in `kernelspec.go`'s package doc why Pi and non-Pi boards use
      different mechanisms, so a future board addition knows which pattern
      to follow
