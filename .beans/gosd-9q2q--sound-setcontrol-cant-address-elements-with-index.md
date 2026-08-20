---
# gosd-9q2q
title: 'sound: SetControl can''t address elements with Index != 0 and its not-found error hides why'
status: completed
type: task
priority: low
created_at: 2026-07-31T07:54:33Z
updated_at: 2026-08-20T13:02:01Z
---

Found by review sweep `gosd-fuxs` (gadget/sound area), verified.

`SetControl` (sound/platform_linux.go:355-378) skips every element with
`Index != 0`, while Control.Index's own doc (control.go:107-108) documents
same-named multi-index elements as real. A card with "Foo" at index 0 and
1 can only ever have index 0 addressed, and the failure reads "card has no
control named %q" — misleading when the name matched at another index.
SetControl is documented as the escape hatch for hardware the audibility
pass gets wrong, so the gap matters exactly when it's needed.

**Fix:** add an indexed variant (or variadic option), and make the
not-found error mention when the name exists at a different index.


## Summary of Changes

Added `findControl(elements, name, index)` (sound/control.go), used by both
the new `Device.SetControlIndexed` and the existing `SetControl` (now a
thin `index=0` wrapper over a shared `setControl`). `SetControlIndexed` is
exposed via a separate `sound.IndexedControl` interface rather than a new
`Device` method, so a fake `Device` written for an app's own tests isn't
forced to implement more than `SetControl`. `findControl`'s not-found error
now distinguishes two cases: the name exists at other indexes (names them,
points at `SetControlIndexed`) versus the name doesn't exist at all (points
at `Device.Mixer`) — covered by
`TestFindControlAddressesElementsByIndex`,
`TestFindControlErrorNamesTheIndexesItActuallyFound`, and
`TestFindControlErrorForANameThatDoesNotExistAtAll`.
