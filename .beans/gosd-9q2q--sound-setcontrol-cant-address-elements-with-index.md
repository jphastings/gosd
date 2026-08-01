---
# gosd-9q2q
title: 'sound: SetControl can''t address elements with Index != 0 and its not-found error hides why'
status: todo
type: task
priority: low
created_at: 2026-07-31T07:54:33Z
updated_at: 2026-07-31T07:54:33Z
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
