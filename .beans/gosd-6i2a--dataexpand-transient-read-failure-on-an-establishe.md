---
# gosd-6i2a
title: 'dataexpand: transient read failure on an established data partition is treated as corruption and halts the device'
status: todo
type: bug
priority: normal
created_at: 2026-07-31T07:53:30Z
updated_at: 2026-07-31T07:53:30Z
---

Found by review sweep `gosd-fuxs` (gosd-init runtime area), verified.

`verifyEstablished` (cmd/gosd-init/internal/dataexpand/dataexpand.go:193-206)
wraps both "device node missing" (checked once, no retry) and "Inspect
read failed" in `ErrDataCorrupt`, which boot.Run routes to
`haltForDataCorruption` → Halt. The creation path polls the node for 5s
(`waitForNode`) precisely because there is no udev to synchronize on; the
established path checks exactly once. This is the only place in gosd-init
where a transient I/O hiccup escalates to a terminal halt — everything
else retries.

**Failure scenario:** an intermittent EIO/EBUSY on an otherwise healthy
card → boot-failure.log tells the owner to reformat partition 2 → device
halts (not reboots). A retry would have succeeded; instead the device
needs a physical visit and the log actively advises destroying good data.

**Fix:** reuse waitForNode's poll shape for both checks; a persistent
*read* failure returns a non-ErrDataCorrupt error so boot falls through to
the read-only /data placeholder. Reserve ErrDataCorrupt + halt for a
successful read that is definitively not a GOSD-DATA FAT32 volume.
