---
# gosd-45bv
title: 'blockmount: concurrent emmc and disk FormatAndMount can both pick and format the same eMMC'
status: todo
type: bug
priority: normal
created_at: 2026-07-31T07:58:52Z
updated_at: 2026-07-31T07:58:52Z
---

Found by review sweep `gosd-fuxs` (storage area), verified (race window
certain; hitting it requires an app running both concurrently).

`blockmount.Run` is MountedAt → Discover → Inspect → Format → Mount with
no lock and no re-check between Discover and Format; both public packages
explicitly encourage backgrounded concurrent use ("returns immediately;
the work runs in the background"). On a Rockchip board booted from SD
with an idle onboard eMMC and nothing attached, disk's candidate list
(deviceClasses ends with "mmcblk") and emmc's (Kind == "MMC") are the
same single device; the InUse exclusion only helps after one side mounts.

**Failure scenario:** app starts `emmc.FormatAndMount("APPDATA",...)` and
`disk.FormatAndMount("BULK",...)` at boot (both doc-encouraged). Both
Discover before either Mounts → both format /dev/mmcblk0 with interleaved
writes → both mount it at two mountpoints with independent vfat
superblocks. Guaranteed corruption — but only when the interleaving lands
badly, so it's intermittent.

**Fix:** package-level mutex serialising blockmount.Run (once-per-boot,
slow ops; contention irrelevant) + a post-Discover re-check of
MountedSources immediately before Format.
