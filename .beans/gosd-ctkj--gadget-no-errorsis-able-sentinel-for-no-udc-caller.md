---
# gosd-ctkj
title: 'gadget: no errors.Is-able sentinel for "no UDC" — callers can''t degrade gracefully like sound/emmc/disk allow'
status: todo
type: task
priority: normal
created_at: 2026-07-31T07:54:33Z
updated_at: 2026-07-31T07:54:33Z
---

Found by review sweep `gosd-fuxs` (gadget/sound area), verified.

sound/emmc/disk each export Err* sentinels so apps can errors.Is and
degrade (ErrNoDevice, ErrNoEMMC, ErrNoDisk...). gadget exports none — the
no-controller error (gadget.go:166) is plain fmt.Errorf. Tell-tale:
examples/usbwebsite reimplements firstUDC/udcState itself
(main.go:383-394) rather than calling Apply and inspecting the error.

**Fix:** export gadget.ErrNoController, wrap it in firstUDC's error, and
switch usbwebsite to the public path as the worked example.
