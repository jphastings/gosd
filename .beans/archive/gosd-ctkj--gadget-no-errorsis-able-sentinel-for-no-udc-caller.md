---
# gosd-ctkj
title: 'gadget: no errors.Is-able sentinel for "no UDC" — callers can''t degrade gracefully like sound/emmc/disk allow'
status: completed
type: task
priority: normal
created_at: 2026-07-31T07:54:33Z
updated_at: 2026-08-20T06:27:33Z
---

Found by review sweep `gosd-fuxs` (gadget/sound area), verified.

sound/emmc/disk each export Err* sentinels so apps can errors.Is and
degrade (ErrNoDevice, ErrNoEMMC, ErrNoDisk...). gadget exports none — the
no-controller error (gadget.go:166) is plain fmt.Errorf. Tell-tale:
examples/usbwebsite reimplements firstUDC/udcState itself
(main.go:383-394) rather than calling Apply and inspecting the error.

**Fix:** export gadget.ErrNoController, wrap it in firstUDC's error, and
switch usbwebsite to the public path as the worked example.

## Summary of Changes

`gadget/gadget.go` now exports `ErrNoController`, wrapped (via `%w`) into the error `firstUDC` (and so `Apply`) returns when no USB peripheral controller is found under `/sys/class/udc` — matching the `errors.Is`-able sentinel convention `sound.ErrNoDevice`, `emmc.ErrNoEMMC`, and `disk.ErrNoDisk` already give callers that want to detect a missing device and degrade gracefully. `gadget` is public, semver-relevant API (per this repo's locked decisions); this is purely additive — no existing exported signature or behavior changed, and the wrapped error's text is unchanged (still names the board-specific fix, e.g. `--usb-gadget`).

`examples/usbwebsite/main.go`'s own local `firstUDC` now wraps the same `gadget.ErrNoController` sentinel instead of a disconnected bespoke string. It keeps its own pre-`Apply` controller lookup (needed to check the cable-attached state and skip an unmount/remount cycle before even trying `Apply` — `Apply`'s error alone can't provide that, since a UDC in "not attached" state binds successfully), but its error now participates in the same `errors.Is` contract a direct `gadget.Gadget.Apply()` caller gets, which is the worked example this bean asked for.

Added a `gadget/gadget_test.go` assertion (in `TestApplyFailsWithNoUDC`) that `Apply()`'s returned error satisfies `errors.Is(err, gadget.ErrNoController)`.

Added `.changeset/actionable-cli-errors.md` (gosd: minor) since this is a public API addition, covering this change alongside gosd-4k5k's and gosd-2maa's user-facing error-message improvements from the same PR.
