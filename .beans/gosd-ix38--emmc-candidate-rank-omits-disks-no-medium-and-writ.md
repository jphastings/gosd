---
# gosd-ix38
title: 'emmc: candidate rank omits disk''s no-medium and write-protected filters — latent emmc/disk semantic divergence'
status: todo
type: task
priority: low
created_at: 2026-07-31T07:59:13Z
updated_at: 2026-07-31T07:59:13Z
---

Found by review sweep `gosd-fuxs` (storage area), verified.

emmc.go:113's rank is `dev.Kind == "MMC"` alone; disk.go:229-239 rejects
SizeSectors==0, ReadOnly, and MMC hardware partitions first. Device
carries those fields precisely for candidate weighing, and the runtime
docs present present-medium/writable as the shared rule. emmc is today
accidentally safe on hardware partitions only via a sysfs-topology quirk
(their parent gendisk has no `type` attribute so Kind is "") — exactly the
implicit coupling CLAUDE.md warns must not silently diverge.

**Fix:** move the SizeSectors==0/ReadOnly rejection into blockmount
(`Usable(Device)` applied inside Candidates) so both packages inherit it
by construction; ranks then express only class preference.
