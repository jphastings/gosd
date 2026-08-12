---
# gosd-e3e3
title: 'diskfmt: FAT32 FAT size under-computed at ~0.8% of volume sizes — 64GiB/128GiB/256GiB volumes fail fsck on macOS'
status: completed
type: bug
priority: high
created_at: 2026-07-31T07:58:28Z
updated_at: 2026-07-31T12:30:00Z
---

Found by review sweep `gosd-fuxs` (storage area). Empirically verified:
the reviewing agent built the real FormatFAT32 path, wrote sparse images,
and ran macOS `fsck_msdos -n`: 16GiB/32GiB/64GiB/256GiB and
maxFAT32Bytes-exactly all report "FAT size too small, N entries won't fit"
(256MiB GOSD-BOOT, 1GiB, 8GiB are clean). Sweep 1-300GiB at 1MiB steps:
2112/261181 sizes defective (~0.8%) — but 28/256 whole-GiB sizes (11%),
i.e. exactly the round sizes real media and the dataexpand 256GiB cap land
on. Shortfall is 1-2 FAT entries.

Root cause: the sectors-per-FAT formula (go-diskfs fat32.go:136, mirrored
byte-identically in internal/diskfmt/fat32limit.go:71-78) uses numerator
`4*(totalSectors-32)`; the spec-correct requirement solves to
`4*(totalSectors-32) + 8*sectorsPerCluster`, so ceil under-rounds when the
division lands near-exact. With the corrected numerator the analytical
sweep shows 0/261181 defective.

Distinct from gosd-8kdm/gosd-mt53 (uint16 truncation past 256GiB) and
gosd-zzdz (readdir): this is under-sizing at ordinary sizes, and
maxFAT32Bytes — the merged guard's own boundary — is itself defective.

**Failure scenario:** `--data-size=64GiB`, or dataexpand growing to its
256GiB cap, or disk.FormatAndMount on a 64GiB stick → BPB advertises 1-2
more clusters than the FAT can index. Linux clamps silently; macOS Disk
Utility First Aid and Windows chkdsk report the volume damaged and offer
a BPB-rewriting repair. End users are told their card is broken.

**Fix:** upstream bug in go-diskfs — per the no-third-party-PRs rule, the
patch (one-line numerator change + rationale above) is recorded here for
JP to send. Local mitigation available now: FormatFAT32 shrinks d.Size to
the largest self-consistent value (costs ≤2 clusters) before calling
CreateFilesystem; keep fat32limit.go's mirror in lockstep either way. Add
an fsck-based CI check (see gosd-zkyg) so this class is
caught mechanically.

## Summary of Changes

### The boundary, derived

Write `N` for a volume's non-reserved sectors (`totalSectors - 32`), `S` for
sectors per cluster and `F` for sectors per FAT. The data area is `N - 2F`
sectors and holds `floor((N-2F)/S)` clusters, and a FAT sector holds 128
32-bit entries, so the volume can address itself only when

    128F >= floor((N-2F)/S) + 2

Solving over the integers, two ways:

* **for F** → `F >= (4N + 8S) / (512S + 8)`. go-diskfs uses numerator `4N`,
  missing the `8S` term — hence the under-rounding.
* **for N** → `N <= F*(128S + 2) - (S + 1)`.

Because `512S + 8 == 4*(128S + 2)`, go-diskfs's `F` is exactly
`ceil(N / (128S+2))`, so `F*(128S+2)` is the top of `F`'s band and **precisely
the top `S+1` sectors of every band are defective**. That gives the observed
`(S+1)/(128S+2)` defect rate — 65/8194 = 0.79% at 32KiB clusters — and bounds
the repair at `S+1` sectors, i.e. ≤2 clusters.

### Local mitigation (this PR)

* New `internal/diskfmt/fat32selfconsistent.go`:
  `LargestSelfConsistentFAT32Bytes(sizeBytes)` returns the largest size ≤
  `sizeBytes` that go-diskfs's *own* formula lays out with an addressable FAT,
  via `fat32SelfConsistentSectorLimit` = `F*(128S+2) - (S+1)` from the
  derivation above. It loops, because trimming can drop a volume into a
  smaller cluster-size class (the classes are size-indexed) and the limit then
  moves; each pass strictly shrinks the candidate, so it settles (≤3 passes
  observed over a full 1MiB-step sweep of 1-300GiB plus sector-granular sweeps
  across every class boundary).
* `fat32limit.go` is **unchanged**: its formulas still mirror what go-diskfs
  actually does, which is what the `MaxFAT32Bytes` ceiling guard needs. The new
  helper derives from that mirror rather than correcting it.
* `FormatFAT32` trims `d.Size` after `checkFAT32Size` (order matters: trimming
  must never turn an oversized device into an accepted one — pinned by a test).
  This covers `emmc`, `disk`/`blockmount` and gosd-init's `dataexpand`
  first-boot format, which all route through it.
* `internal/image`'s `computeLayout` trims the GOSD-DATA partition the same
  way — that path calls go-diskfs directly, so it needed its own call.
  GOSD-BOOT's fixed 256MiB is already self-consistent and is now pinned as such
  by a test.
* `MaxFAT32Bytes` interaction confirmed: the ceiling was itself defective
  (`N == 65535*8194` exactly, a band top). It now trims by 65 sectors and
  still lays out at `F == 65535`, so the guard admits it and it formats
  correctly. A device one sector past the ceiling still trims within its own
  band, keeps `F == 65536`, and is still refused.

**Observed shrink:** 0 bytes for ~99.2% of sizes; worst case 65 sectors
(33,280 bytes) at 32KiB clusters — 16GiB loses 8,704 bytes, 64GiB 20,992,
256GiB and `maxFAT32Bytes` 33,280 each. Never more than two clusters of the
requested size, verified by sweep and asserted in tests.

**Tests:** `internal/diskfmt/fat32selfconsistent_test.go` formats sparse
backing files at 256MiB/1GiB/8GiB (healthy) and 16/32/64/256GiB +
`maxFAT32Bytes` (all previously defective), reads the BPB back off the device
and applies the same arithmetic `fsck_msdos` does; plus a boundary unit test
walking `lastGood-1 … bandTop` in every cluster class, and a whole-GiB sweep.
`internal/image` writes an image with a 64MiB data partition (a defective
size) and checks both partitions' BPBs. With the mitigation stubbed out, those
tests fail at exactly the sizes this bean names — the defect is reproduced, not
just asserted away.

### Upstream patch for go-diskfs (NOT sent — for JP to decide)

`github.com/diskfs/go-diskfs` v1.9.3, `filesystem/fat32/fat32.go:132-136`:

```diff
 	// Closed-form equivalent of the dosfstools mkfs.fat sectors-per-FAT search:
 	// smallest X such that (reserved + 2X + clusters*SPC) == totalSectors and
 	// X * (bytesPerSector/4) >= clusters + 2.
+	// The second condition contributes the 8*sectorsPerCluster term: solving
+	// X*(bytesPerSector/4) >= (totalSectors-reserved-2X)/SPC + 2 for X gives
+	// X >= (4*(totalSectors-reserved) + 8*SPC) / (bytesPerSector*SPC + 8).
+	// Without it the ceiling under-rounds whenever the division lands within
+	// one cluster of exact, and the volume advertises one or two more clusters
+	// than either FAT can index.
 	fatEntryDenom := uint32(blocksize)*uint32(sectorsPerCluster) + 8
-	sectorsPerFat := uint16((4*(totalSectors-uint32(reservedSectors)) + fatEntryDenom - 1) / fatEntryDenom)
+	sectorsPerFat := uint16((4*(totalSectors-uint32(reservedSectors)) + 8*uint32(sectorsPerCluster) + fatEntryDenom - 1) / fatEntryDenom)
```

Correctness of the patched form: `F' = ceil((N + 2S) / (128S + 2))`, and
self-consistency needs `F' >= (N + S + 1) / (128S + 2)`, which holds for every
`S >= 1`. An analytical sweep of 1-300GiB (1MiB steps, plus sector-granular
windows around 16/32/64/128/256GiB) finds 0 defective sizes with the patch
against 2,471 without.

Two caveats worth sending with it: the patch can raise `sectorsPerFat` by one,
which makes the pre-existing `uint16` truncation on that same line bite one
sector earlier (separate defect — see gosd-8kdm/gosd-mt53, which are about
volumes past ~256GiB, not about under-sizing at ordinary sizes); and go-diskfs
has no regression test in this area, so a table test over the round sizes
would be worth including.
