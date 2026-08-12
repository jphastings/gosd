---
# gosd-0r40
title: 'gadget: failed Apply strands configfs state; supervisor restart then hits EEXIST until reboot'
status: completed
type: bug
priority: high
created_at: 2026-07-31T07:50:14Z
updated_at: 2026-07-31T07:50:14Z
---

Found by review sweep `gosd-fuxs` (gadget/sound area), verified against the
code.

`Gadget.apply` (gadget/gadget.go:75-97) runs `materialize()` — which writes
the whole configfs tree: identity files, strings, config c.1, function dirs,
and the function symlinks — and only sets `g.fs` after the *UDC bind* also
succeeds. If `firstUDC` fails (no controller: boot race, or an image built
without `--usb-gadget`, the exact case the package doc calls out) or the UDC
write fails, `apply` returns with `g.fs == nil`, and `Close()`
(gadget.go:178-180) is then a documented no-op. The already-materialized
configfs tree has no teardown path.

**Failure scenario:** `examples/usbserial` exits non-zero when `Apply` fails;
gosd-init's supervisor restarts it. The fresh process's `Apply` re-runs
`materialize()`: `MkdirAll`/`WriteFile` are idempotent, but
`Symlink(funcDir, .../configs/c.1/<fn>)` (gadget.go:153) returns a real
`EEXIST` — configfs is kernel-resident and survives process restarts. From
then on every retry fails at the symlink, and the gadget is neither applied
nor cleanable until the board reboots.

**Fix:** on any error after `materialize()` starts, unwind via the same
canonical teardown `Close()` uses (or set `g.fs` immediately after a
successful `materialize()` so `Close()` works even when the UDC bind failed
— pick one; the first keeps the "failed Apply needs no Close" contract).

**Test prerequisite:** `fakeFS.Symlink` (gadget/fakes_test.go:93-100)
silently overwrites an existing link, unlike real `os.Symlink` (EEXIST), so
the regression test for this can't currently be written against the fake.
Make the fake return an `fs.ErrExist`-wrapped error on duplicate symlink
first, then pin the apply→fail-after-materialize→re-apply sequence.

## Todos

- [x] Make fakeFS.Symlink model EEXIST like os.Symlink
- [x] Unwind materialized configfs state when apply fails before/at UDC bind
- [x] Regression test: apply fails at UDC step, second apply succeeds after fix

## Summary of Changes

`gadget/fakes_test.go`'s `fakeFS.Symlink` now fails with an `fs.ErrExist`-
wrapped error when `newname` is already occupied by a link, directory, or
file — matching real `os.Symlink`/configfs instead of silently overwriting.
Without this the regression test below couldn't detect a stranded symlink
from a prior failed `Apply`.

`gadget/gadget.go`: the canonical configfs teardown sequence `Close()` used
(gosd-cjs2's fix — unlink each function from `configs/c.1`, remove the
config's `strings/0x409` and the config itself, remove each function's
directory, then the gadget's `strings/0x409` and the gadget root, skipping
kernel-owned default groups) is now factored out into a package-level
`removeConfigfsTree(fsys writableFS, fns []Function) error`, called from both
`Close()` and a new `(g *Gadget) failApply(fsys, applyErr)` helper. `apply()`
calls `failApply` on every error path after `materialize()` begins — a
partial `materialize()` failure, a `firstUDC` failure, or a UDC-write
failure — so any already-materialized configfs state is unwound before the
original error is returned. `removeConfigfsTree`'s attempt-every-step,
collect-first-error shape (unchanged from `Close()`) tolerates a partially
materialized tree without extra logic: a step whose node doesn't exist yet
just contributes a discarded `fs.ErrNotExist` to the (also discarded) unwind
error, and every later step still runs. No UDC bind ever succeeds on a path
that reaches `failApply`, so, unlike `Close()`, there's nothing to unbind
first. The documented "a failed Apply needs no Close" contract is preserved
by design: unwinding immediately means there's nothing left for a `Close()`
call to find. `Close()`'s own behavior (ordering, returned error, the
gosd-cjs2 regression test) is unchanged — it now just delegates to the
shared helper instead of inlining the sequence.

`gadget/gadget_test.go` gained:
- `TestFakeFSSymlinkFailsOnExistingTarget`, pinning the fake's new EEXIST
  behavior directly.
- `TestApplyUnwindsOnMissingUDC` — the bean's regression test: `Apply()`
  fails with no UDC present, `assertNoGadgetState` confirms every file,
  symlink, and directory under `gadgetRoot` is gone, then a second `Apply()`
  (on a fresh `Gadget`, once a UDC is seeded) succeeds — without the fix this
  hits `EEXIST` on the first function's config symlink.
- `TestApplyUnwindsPartialMaterializeFailure`, covering the other failure
  shape called out in the bean: `materialize()` itself failing mid-tree (a
  second function's `Create` erroring after the first function was fully
  created and linked) also leaves no configfs state behind.

All three new tests were confirmed to fail against the pre-fix code (stashed
the production and fake changes, kept the tests) before being reinstated,
so they're genuine regressions, not tautologies.
