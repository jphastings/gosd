---
# gosd-8rw2
title: 'diskfmt: Inspect reports FAT12/FAT16 volumes as FAT32 — refusal messages name a filesystem that isn''t there'
status: todo
type: task
priority: low
created_at: 2026-07-31T07:59:13Z
updated_at: 2026-07-31T07:59:13Z
---

Found by review sweep `gosd-fuxs` (storage area), verified.

diskfmt.go:156-165: isFAT accepts TypeFat32|TypeFat16|TypeFat12 but the
Contents returned always says FS: FAT32. A FAT16 stick full of user data
gets refused with "already holds FAT32 labelled ..." — the app author's
only diagnostic, and it lies. Also makes Contents.FS useless for any
future width-sensitive decision.

**Fix:** either add FAT16/FAT12 FS values (all MountType "vfat") or
report the honest string "FAT". Behavioral test: Inspect of a FAT16 image
reports something a user would recognise.
