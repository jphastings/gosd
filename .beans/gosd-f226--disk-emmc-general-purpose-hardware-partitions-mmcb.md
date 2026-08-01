---
# gosd-f226
title: 'disk: eMMC general-purpose hardware partitions (mmcblkNgpM) are valid format targets — vendor GP partitions can be wiped'
status: todo
type: bug
priority: normal
created_at: 2026-07-31T07:58:52Z
updated_at: 2026-07-31T07:58:52Z
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
