---
# gosd-zldw
title: internal/blockmount.Candidates keeps the boot disk off the format list only by the mount table — the same accident gosd-ix0r removed for gadget
status: todo
type: bug
priority: high
created_at: 2026-08-21T08:03:47Z
updated_at: 2026-08-21T08:03:47Z
---

Split out of gosd-ix0r, which fixed the same accident one layer up.

`internal/blockmount.Candidates` (discover.go) skips a device when it, or
any of its partitions, appears as a mount source — and its own comment says
what that is relied on for: "Anything mounted from a device ... makes it in
use, **which is what keeps the media the board booted from off the list**".

That is exactly the reasoning gosd-ix0r found wanting in
`gadget.MassStorage`: it holds today only because gosd-init keeps /boot
mounted for the life of the device, not because any package enforces it. The
consequence here is worse than a disclosure — `disk.FormatAndMount` and
`emmc.FormatAndMount` pick their target through this function, so a
candidate that happened not to be mounted at that moment would be
**formatted**, taking the kernel and config tree with it.

## The mechanism now exists

gosd-ix0r added `internal/devreserve`: gosd-init publishes the block devices
GoSD reserves at `/run/gosd/reserved-devices.json`, and
`devreserve.Reservations.Exposes(candidate)` answers "would handing out this
device hand out one of ours?". `blockmount.Candidates` can consult the same
list alongside `InUse`/`Usable`, which makes the exclusion a rule rather
than a coincidence in the second place it matters.

Note the direction of the check differs slightly from the gadget case: a
format target is destructive, so a candidate that is a *partition of* a
reserved device's disk is arguably also unsafe to hand to `mkfs` in the
whole-disk formatting path. Decide that explicitly rather than reusing
`Exposes` without thought.

## Relationship to gosd-onjv

gosd-onjv already carries a todo to keep its new config partition out of
`disk.Devices`, `disk.rank`/`Usable`, `FormatAndMountDevice` and
`emmc.chooseEMMC`. That todo and this bean want the same seam; whichever
lands first should build it, and the other consume it.

## Todos

- [ ] Decide whether a reserved device excludes its whole disk from format candidacy, or only itself
- [ ] Consult the reserved-device list in `blockmount.Candidates`, alongside (not instead of) the existing `InUse` check
- [ ] Tests covering a reserved device that is NOT currently mounted
- [ ] Confirm `emmc.chooseEMMC`'s single-device selection gets the same treatment
