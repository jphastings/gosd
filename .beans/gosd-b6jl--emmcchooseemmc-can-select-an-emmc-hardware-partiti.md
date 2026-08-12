---
# gosd-b6jl
title: 'emmc.chooseEMMC can select an eMMC hardware partition: the exclusion disk.rank has is missing here'
status: todo
type: bug
created_at: 2026-08-12T04:18:42Z
updated_at: 2026-08-12T04:18:42Z
---

**Severity: Medium.** Low likelihood today, but the blast radius is an
unrecoverable brick, and the only thing preventing it is an undocumented
kernel behaviour nobody has verified on the boards this project ships.

## Verified

`disk.rank` excludes an eMMC's `boot0`/`boot1`/`rpmb`/`gp0-gp3` hardware
partitions explicitly, via `isMMCHardwarePartition`'s regex
(`disk/disk.go:294-317`) — added by bean gosd-f226 after review found that
`disk.FormatAndMount(destructive=true)` could otherwise wipe a GP hardware
partition holding vendor keys or calibration data.

`emmc.chooseEMMC` (`emmc/emmc.go:251-276`) has **no equivalent exclusion**.
Its ranking is:

```go
rank := func(dev) (int, bool) { return 0, dev.Kind == "MMC" }
```

It relies entirely on the empirical fact that a hardware partition's gendisk
reports no `device/type` sysfs attribute, so `Kind == ""` rather than
`"MMC"`. The code acknowledges this in its own words as an accidental-safety
quirk, and the referenced follow-up (gosd-ix38) explicitly scoped closing it
**out** of its fix. No open bean currently closes it.

## Why this is not acceptable as-is

The protection is: (a) not guaranteed by any documented kernel contract,
(b) unverified across the Rockchip family this project ships (Radxa Zero 3E,
NanoPi Zero2, ROCK 4SE), and (c) silent if it regresses — a future kernel bump
that populates `device/type` for these gendisks turns `emmc.FormatAndMount`
into a formatter of boot code or RPMB with no warning and no error. Losing
boot0/boot1 means the board no longer boots at all and cannot be recovered
from the SD card.

Two packages that share `internal/blockmount` and are documented as differing
only in candidate *selection* should not differ on whether they can destroy a
hardware partition.

## Fix

Port `disk.isMMCHardwarePartition`'s regex exclusion into `emmc.chooseEMMC`.
Better: move it into `internal/blockmount` so both packages inherit it
structurally — mirroring how gosd-ix38 already centralised the
present-medium and write-protected checks into `blockmount.Usable`.

## Todos

- [ ] Exclude MMC hardware partitions in `emmc.chooseEMMC` by explicit pattern, not by the Kind quirk
- [ ] Prefer centralising the exclusion in `blockmount` so `disk` and `emmc` cannot diverge again
- [ ] Test: a candidate list containing boot0/boot1/rpmb/gp0 never selects one, regardless of reported Kind
- [ ] Update `internal/blockmount`'s package doc, which currently records the divergence as known and accepted
