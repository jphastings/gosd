---
# gosd-ix38
title: 'emmc: candidate rank omits disk''s no-medium and write-protected filters — latent emmc/disk semantic divergence'
status: completed
type: task
priority: low
created_at: 2026-07-31T07:59:13Z
updated_at: 2026-08-07T17:30:00Z
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

## Summary of Changes

Added `blockmount.Usable(Device) bool` (`internal/blockmount/discover.go`):
`SizeSectors != 0 && !ReadOnly`. `Candidates` now checks it for every device,
ahead of calling the caller's `Rank`, so no-medium and write-protected
devices can never become a candidate regardless of what any package's rank
function says — the check can no longer be silently omitted by one package
and not the other.

`disk.rank` (`disk/disk.go`) had its `SizeSectors == 0 || ReadOnly` clause
removed — now redundant with `Usable` — keeping only the eMMC
boot/RPMB/GP hardware-partition exclusion, which is disk's own genuine
difference (not a present-medium/writable rule). `emmc.chooseEMMC`'s rank
(`dev.Kind == "MMC"` alone) is unchanged in code, but now inherits the same
present-medium/writable exclusion through `blockmount.Choose`/`Candidates` —
closing the gap the bean found, where it was previously safe against a
write-protected or medium-less MMC device only by accident (never by an
actual check). Updated both packages' doc comments to describe the new
split: rank expresses class preference only; blockmount enforces
present-medium/writable, once, for both. The MMC-hardware-partition sysfs
quirk emmc's rank still depends on (Kind reads "" rather than "MMC" for
boot/rpmb/gp partitions) is unchanged and out of this bean's scope — it is
its own accidental-safety gap, already tracked by gosd-f226's follow-up note.

Tests added:
- `internal/blockmount`: `TestUsable` (table test over present/no-medium/
  write-protected/both), and
  `TestCandidatesExcludesNoMediumAndWriteProtectedRegardlessOfRank`, which
  uses `acceptAll` (a rank that accepts everything) to prove the exclusion
  is enforced by `Candidates` itself, not by any package's rank remembering
  to make it.
- `emmc`: `TestChooseEMMCRejectsNoMediumOrWriteProtected` — the key
  regression test. It constructs an MMC-typed device with `SizeSectors: 0`
  and, separately, one with `ReadOnly: true`, and asserts `chooseEMMC`
  reports `ErrNoEMMC` for both. Before this fix, `chooseEMMC`'s
  `dev.Kind == "MMC"`-only rank would have accepted either as a format
  target.
- `disk`: `TestRankLeavesMediumAndWriteProtectionToBlockmount` pins the
  responsibility split explicitly — `rank` alone now accepts a no-medium or
  write-protected device (`ok == true`), proving that filtering moved to
  `blockmount.Usable` rather than merely being duplicated. Disk's existing
  end-to-end coverage (`TestChooseReportsErrNoDisk`'s no-medium/
  write-protected cases) is unchanged and still passes, now exercising the
  shared path instead of `disk.rank` directly.
- Fixed pre-existing `blockmount`/`emmc` test fixtures that constructed
  `Device` literals with a zero `SizeSectors` (harmless before, since
  nothing filtered on it in those tests) to carry a real `SizeSectors` via a
  `present()` helper (mirroring `disk_test.go`'s existing one), so they
  don't spuriously fail now that `Usable` is enforced unconditionally.

No `COMPATIBILITY.md` change: no board or feature status moved — this closes
a latent internal safety gap between two already-shipped packages.
