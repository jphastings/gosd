---
# gosd-83sw
title: 'sound: applyAudibility aborts the whole pass on the first control-write failure, skipping independent later fixes'
status: todo
type: task
priority: normal
created_at: 2026-07-31T07:53:30Z
updated_at: 2026-07-31T07:53:30Z
---

Found by review sweep `gosd-fuxs` (gadget/sound area), verified.

`applyAudibility` (sound/control_linux.go:355-378) returns on the first
`c.write` failure, never attempting the remaining changes in the pass.
`unmute` already treats the pass as best-effort/non-fatal to Open, but one
transiently-failing element (DAPM power race, momentarily inactive
control) suppresses unrelated later changes — e.g. a volume raise that
would have succeeded — leaving the board quieter than needed for a reason
unconnected to the failure. Codec-silence layering is exactly what the
pass exists for (see gosd-cfkd's ES8316 findings: multiple independent
mute points).

**Fix:** attempt every change, collect failures with `errors.Join`, return
the joined error alongside the full done list (the API already returns
partial progress).
