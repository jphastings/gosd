---
# gosd-f226
title: 'disk: eMMC general-purpose hardware partitions (mmcblkNgpM) are valid format targets — vendor GP partitions can be wiped'
status: completed
type: bug
priority: normal
created_at: 2026-07-31T07:58:52Z
updated_at: 2026-08-02T10:47:45Z
---

Found by review sweep `gosd-fuxs` (storage area), verified against the
kernel MMC block driver's gendisk naming (boot0/boot1/rpmb/gp0-gp3 are
each independent /sys/block entries).

isMMCHardwarePartition (disk/disk.go:249-256) excludes only
boot0/boot1/rpmb. A GP partition (mmcblk0gp0) is its own /sys/block entry
— so InUse via readPartitions("mmcblk0") never covers it, it ships
force_ro=false so the ReadOnly filter passes it, and it matches the
"mmcblk" class prefix → eligible, and on a board whose main eMMC is
excluded it can be the CHOSEN device.

**Failure scenario:** Rockchip board booted from a vendor-configured eMMC
with a GP partition (typical contents: DRM keys, calibration, secure
storage). disk.FormatAndMount(destructive=true) writes whole-device FAT32
over it. No way back.

**Fix:** extend the suffix list to gp0-gp3, or better match structurally:
reject any name matching `mmcblk\d+(boot\d|rpmb|gp\d)`. Bean gosd-yggd
documented the exclusion intent; the GP class just wasn't known then.

## Summary of Changes

`isMMCHardwarePartition` (`disk/disk.go`) now matches structurally, via
`regexp.MustCompile("^mmcblk\\d+(boot\\d+|rpmb|gp\\d+)$")`, instead of a
suffix list, so `mmcblk0gp0`-`mmcblk0gp3` (and any unexpected index the kernel
does not use today, matched defensively) are rejected alongside boot0/boot1/
rpmb, and a plain device or ordinary partition (`mmcblk0`, `mmcblk0p1`) is
provably never a false positive.

Added a table-driven `TestIsMMCHardwarePartition` covering the real
hardware-partition shapes, double-digit device numbers, plain
devices/partitions, and adversarial near-misses, plus a discovery-level
`TestChooseNeverPicksAnEMMCGeneralPurposeHardwarePartition` that reproduces
the bean's exact failure scenario (booted eMMC + GP partition) and fails
against the pre-fix suffix list (verified by temporarily reverting
`disk/disk.go` and re-running).

Checked `emmc`'s candidate rank (`chooseEMMC`, `dev.Kind == "MMC"`): it does
not need the same explicit guard today — a GP/boot/rpmb hardware partition's
gendisk has no `device/type` sysfs attribute, so `Kind` reads `""`, not
`"MMC"`, and it is already excluded by the existing check. Documented this as
an accidental-safety quirk in a comment on `chooseEMMC`, added
`TestChooseEMMCIgnoresGeneralPurposeHardwarePartitions` to pin it, and pointed
at `gosd-ix38` (unchanged scope) as the bean that folds this into an explicit,
non-quirk-dependent shared check.

No COMPATIBILITY.md change: no board or feature status moved, this closes a
data-destruction bug in an existing allowlist.
