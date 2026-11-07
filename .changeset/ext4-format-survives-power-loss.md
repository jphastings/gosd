---
gosd: patch
---

#### An interrupted ext4 format no longer leaves a drive stuck

If power is lost while `disk` or `emmc` is formatting a drive as ext4, the drive now reads as blank on the next boot and is simply formatted again. Previously it was left holding a half-written volume that every later mount refused. The first megabyte, which holds the filesystem's superblock and label, is now cleared before anything else is written and filled in last.

Separately, a volume whose only content is a non-empty `lost+found` (what `fsck` leaves behind after rescuing files) now counts as holding data. It's no longer mistaken for the remains of an unfinished format and wiped.
