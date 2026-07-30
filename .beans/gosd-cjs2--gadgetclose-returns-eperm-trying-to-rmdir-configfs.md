---
# gosd-cjs2
title: gadget.Close() returns EPERM trying to rmdir configfs default groups
status: todo
type: bug
priority: normal
created_at: 2026-07-30T07:44:22Z
updated_at: 2026-07-30T07:44:22Z
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

- `Close()` returns nil after a normal apply/close cycle on real hardware.
- No configfs state is stranded under `/sys/kernel/config/usb_gadget/gosd`
  after close (verify on a board; betamin's repeated apply/close on every
  playback boot is a convenient exerciser).
- `go test ./...` covers the nil-error case with a fake.
