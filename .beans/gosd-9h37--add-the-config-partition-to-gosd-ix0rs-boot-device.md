---
# gosd-9h37
title: Add the config partition to gosd-ix0r's boot-device exclusion set
status: todo
type: task
priority: high
created_at: 2026-08-21T08:14:16Z
updated_at: 2026-08-21T08:14:16Z
parent: gosd-onjv
blocked_by:
    - gosd-ix0r
---

The config partition (bean gosd-onjv) must not be reachable from an app's
storage enumeration: an app that picks it up and publishes it as a
mass-storage LUN re-creates, structurally, the exact failure gosd-onjv exists
to prevent.

gosd-ix0r owns the mechanism — gosd-init publishing the identity of the
device it booted from, and `gadget.MassStorage.Create` refusing those devices
— so this is deliberately layered on top of it rather than built as a second,
parallel exclusion in `disk` and `emmc` (two things to keep in step, one of
which will silently drift).

## Todo

- [ ] Add the config partition's node to whatever set gosd-ix0r publishes
- [ ] Check `disk.Devices`, `disk.rank`/`Usable` and `FormatAndMountDevice`
      against it
- [ ] Check `emmc.chooseEMMC` against it
- [ ] A test that proves an app cannot enumerate the config partition

Blocked on gosd-ix0r landing. If gosd-onjv's second PR is still open when
that happens, fold this into it instead.
