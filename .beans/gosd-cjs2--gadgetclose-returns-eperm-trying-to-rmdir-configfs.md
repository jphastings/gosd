---
# gosd-cjs2
title: gadget.Close() returns EPERM trying to rmdir configfs default groups
status: completed
type: bug
priority: normal
created_at: 2026-07-30T07:44:22Z
updated_at: 2026-07-30T22:08:18Z
---

## Symptom

Found via betamin on real hardware (rock-4se, 2026-07-29). Every teardown of
a `gadget.Gadget` reports an error, even when the teardown actually succeeds:

```
modeselect: closing the unconfigured USB gadget: remove
  /sys/kernel/config/usb_gadget/gosd/configs/c.1/strings: operation not permitted
```

This fires on any `Close()` — betamin hits it on every playback boot, because
`internal/modeselect` applies the mass-storage gadget, waits to see whether a
host configures it, and closes it again when none does.

## Cause

`gadget.Close()` (`gadget/gadget.go`) removes every directory `materialize()`
created "via MkdirAll, in reverse (leaves first)", on the stated assumption
that MkdirAll's intermediate directories "each still ha[ve] to be rmdir'd
individually".

That assumption is wrong for configfs. `configs/`, `functions/` and
`strings/` under the gadget, and `strings/` under a config, are **default
groups created by the kernel's gadget driver**, not by our MkdirAll — MkdirAll
found them already present. configfs refuses to rmdir a default group, so each
of those four removals returns EPERM:

- `configs/c.1/strings`  <- the one reported (first error wins)
- `configs`
- `functions`
- `strings`

The canonical configfs gadget teardown sequence removes only the
user-created nodes: unbind UDC, remove the function symlinks from
`configs/c.1/`, `rmdir configs/c.1/strings/0x409`, `rmdir configs/c.1`,
`rmdir functions/<fn>`, `rmdir strings/0x409`, `rmdir <gadget>`. Removing the
gadget directory tears its default groups down for us.

## Impact

Mostly cosmetic *today* — `Close()` continues past the first error by design,
so the final `rmdir <gadget>` still runs and the gadget does come down. But:

- `Close()` always returns a non-nil error, so callers cannot distinguish a
  spurious EPERM from a genuine teardown failure. It trains callers to ignore
  the error.
- It leaves a misleading, alarming line in every serial log.
- Worth confirming on hardware whether the final `rmdir <gadget>` really does
  succeed in this state, or whether some configfs state is being stranded
  across a mode switch (betamin now applies and closes a gadget on every
  playback boot, so any leak would accumulate).

## Proposed approach (not locked)

- Stop attempting to remove the four default groups; remove only
  user-created nodes, matching the canonical sequence above.
- Keep the continue-past-errors behaviour and the first-error return.
- Extend `gadget_test.go`'s fake so it can model a directory that refuses
  removal, and assert `Close()` returns nil on a successful teardown — the
  regression this bean is really about.
- Update the doc comment, which currently states the incorrect rationale
  about MkdirAll intermediates.

## Acceptance

- [ ] `Close()` returns nil after a normal apply/close cycle on real hardware.
- [ ] No configfs state is stranded under `/sys/kernel/config/usb_gadget/gosd`
  after close (verify on a board; betamin's repeated apply/close on every
  playback boot is a convenient exerciser).
- [x] `go test ./...` covers the nil-error case with a fake.



## Summary of Changes

`gadget/gadget.go`'s `Close()` now implements the canonical configfs gadget
teardown sequence instead of walking every directory `materialize()`'s
`MkdirAll` calls touched: unbind UDC, remove each function's symlink from
`configs/c.1/`, `rmdir configs/c.1/strings/0x409`, `rmdir configs/c.1`,
`rmdir functions/<fn>` per function, `rmdir strings/0x409`, `rmdir
<gadget>`. It no longer attempts `rmdir` on `configs`, `functions`, `strings`
(under the gadget) or `configs/c.1/strings` — those are kernel-created
configfs default groups, torn down automatically when their parent
(`configs/c.1` or the gadget root) is removed.

`gadget/fakes_test.go`'s `fakeFS` previously only modeled `lun.0`
(f_mass_storage's default group) as kernel-owned; it now also marks
`strings`, `configs` and `functions` as default groups the instant the
gadget root is created, and a config's `strings` the instant `configs/c.<n>`
is created — matching the real kernel's behavior (a direct `Remove` on any
of them now fails with `fs.ErrPermission`, just like real configfs). Without
this fake fix the old (buggy) `Close()` code passed its own tests, because
the fake didn't model the EPERM the bug depends on.

`gadget/gadget_test.go` gained
`TestCloseRemovesOnlyUserCreatedNodesInCanonicalOrder`, which asserts the
exact ordered sequence of `Remove` calls `Close()` issues and that it
returns nil — confirmed to fail against the pre-fix `Close()` (5 existing
tests also failed once the fake alone was corrected, before the production
fix was reapplied).

**Open question 1 (does the final `rmdir <gadget>` succeed?) — answered from
the code path:** by the time `Close()` reaches `rmdir <gadget>`, every
user-created descendant has already been removed (function symlinks, each
config's `strings/0x409`, `configs/c.1` itself, each `functions/<fn>`, and
the gadget's own `strings/0x409`), so `configs`, `functions` and `strings`
under the gadget are themselves empty default groups at that point and
`rmdir <gadget>` cascades them away for free — this is a code-path
determination, not a hardware measurement.

**Open question 2 (could configfs state strand across a mode switch?) — NOT
settled from code**, so left as an open bench item below rather than
asserted: `Close()`'s continue-past-errors design means if an *earlier*
removal step in the sequence itself fails for a real reason (not the
default-group EPERM this bean fixes), later steps that depend on that node
being gone (e.g. `rmdir configs/c.1` while a function symlink still lingers
inside it) would also fail, and the final `rmdir <gadget>` would then fail
too — genuinely stranding state. This can only be assessed by exercising a
real failure mode on hardware, so it stays unconfirmed.
