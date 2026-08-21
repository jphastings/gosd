---
# gosd-83sw
title: 'sound: applyAudibility aborts the whole pass on the first control-write failure, skipping independent later fixes'
status: completed
type: task
priority: normal
created_at: 2026-07-31T07:53:30Z
updated_at: 2026-08-20T13:01:43Z
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


## Summary of Changes

`applyAudibility` now delegates to a new `applyChanges` (sound/control.go),
which attempts every change in the pass regardless of earlier failures and
returns the changes that succeeded plus every failure joined with
`errors.Join`, each wrapped with the failing `Change`'s own diagnostic
string (element name/numid/target value) so the joined error names every
control that failed and why. `applyChanges` takes a small `controlWriter`
interface so the collect-and-continue behaviour is tested with a fake
(`TestApplyChangesContinuesPastAFailingWrite`,
`TestApplyChangesReportsNoErrorWhenEverySuccessfulWrite`) without needing
real hardware. `Device.Mixer()`/`OpenWith`'s existing `d.changed`/`d.mixerErr`
plumbing was already best-effort and non-fatal to `Open`, so no caller-facing
behaviour changed there beyond the error now covering every failed control
instead of just the first.
