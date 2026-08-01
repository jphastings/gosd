---
# gosd-fnh8
title: 'gadget: MassStorage accepts a currently-mounted backing device — the documented expose-or-mount-never-both rule is unenforced'
status: todo
type: bug
priority: normal
created_at: 2026-07-31T07:58:52Z
updated_at: 2026-07-31T07:58:52Z
---

Found by review sweep `gosd-fuxs` (storage area), verified.

MassStorage.Create (gadget/massstorage.go:36-54) validates only that Path
is non-empty and writes it into lun.0/file; nothing consults
/proc/mounts. The only guard is prose in three doc comments — and the
dangerous call is the docs' own two snippets concatenated:
FormatAndMount returns res.BlockDevice, and handing that to MassStorage
while it's still mounted has the board's vfat page cache and the USB
host's raw writes corrupting the volume with no error anywhere.

**Failure scenario:** developer follows the disk + gadget examples,
forgets the Unmount line → intermittent corruption of the shared volume.

**Fix:** MassStorage.Create (or Apply) rejects a Path present in — or a
partition of a device present in — blockmount.MountedSources(), with an
actionable error naming the mountpoint and the Unmount step. The check is
a /proc/mounts read blockmount already implements. Adjacent: gosd-k2fs
(mass-storage scope, locked) — this adds enforcement, not scope.
