---
# gosd-8kdm
title: go-diskfs FAT32 formatter silently corrupts volumes past ~256GiB (uint16 sectors-per-FAT)
status: in-progress
type: bug
priority: normal
created_at: 2026-07-30T20:28:52Z
updated_at: 2026-08-20T05:53:54Z
---

Found while designing --data-size=expand (bean gosd-6sac): go-diskfs v1.9.3's `fat32.Create` computes `sectorsPerFat := uint16((4*(totalSectors-reserved) + fatEntryDenom - 1) / fatEntryDenom)` — a straight uint16 cast. With the 32KiB clusters the >32GB size class uses, the value exceeds 65535 once the volume passes roughly 256GiB, silently truncating: the FAT is laid out far too small for the cluster count and the resulting filesystem is corrupt. `Fat32MaxSize` (2TiB) doesn't catch it.

Reachable today through anything that formats large media via `internal/diskfmt.FormatFAT32`: the public `disk` package with a 512GB/1TB SSD or USB drive attached, and (mitigated) `--data-size=expand`, which caps its created partition at 256GiB with a logged notice (see `cmd/gosd-init/internal/dataexpand`'s `maxPartitionBytes`) until this is fixed.

Fix options: upstream PR to go-diskfs making the sectors-per-FAT computation 32-bit (FATSz32 is a u32 field on disk — uint16 is purely an implementation slip), then bump the pin and delete both caps; or guard in `diskfmt.FormatFAT32` with an actionable error above the safe size so `disk`/`emmc` callers at least fail loudly instead of corrupting. The exFAT path is unaffected (our own formatter, 32-bit FAT length, validated).

The safe boundary, exactly: truncation begins when totalSectors exceeds ~537,133,000 (≈256.1GiB); 256GiB even (536,870,912 sectors) is within range.


## The boundary, exactly

A volume this large gets 32 KiB clusters (64 sectors), which cost
`512*64 + 8 = 32776` bytes of data plus FAT entries, so the largest volume whose
per-FAT sector count still fits go-diskfs's uint16 is

```
65535 * 32776 / 4 + 32 reserved = 536,993,822 sectors
                                = 274,940,836,864 bytes = 256.06 GiB
```

One sector more needs a 65536th FAT sector. (The ~537,133,000 figure above was
approximate; this is the exact value, derived in `internal/diskfmt/fat32limit.go`
from the same arithmetic rather than hard-coded. 256 GiB even — 536,870,912
sectors, 65520 sectors per FAT — is comfortably inside it.)

There is a **second** overflow above it, missed in the original diagnosis: the
numerator is uint32 arithmetic, and `4*(totalSectors-32) + 32775` passes 2^32 at
~511.996 GiB, wrapping to a tiny or zero count. Measured against v1.9.3 by
formatting sparse whole devices and reading FATSz32 back out of the boot sector:

| device | sectors per FAT recorded | actually needed | outcome |
|---|---|---|---|
| 64 GiB card | 16380 | 16380 | fine |
| 256 GiB | 65520 | 65520 | fine |
| 256.06 GiB (the limit) | 65535 | 65535 | fine |
| the limit + 1 sector | — | 65536 | **panic**: `index out of range [2] with length 1` |
| 300 GiB | 11246 | 76782 | silently corrupt — the FAT addresses 15% of the volume |
| 512 GB SSD | 56505 | 122041 | silently corrupt |
| 512 GiB | — | 131041 | **panic** (uint32 numerator wrap) |
| 1 TB drive | 41834 | 238410 | silently corrupt |

So past the limit it is silent corruption up to ~512 GiB, and an unrecoverable
panic inside the library above that — in an app on a board, a crash with no
diagnosis.

(Unrelated nit spotted while mirroring the arithmetic: the closed form omits the
two reserved FAT entries — the correct numerator is `4*(T-R) + 8*SPC` — so on
large volumes the last one or two clusters have no FAT entry. Linux clamps the
cluster count when mounting, so it costs at most 64 KiB of capacity; not worth a
change of ours, noted below for whoever sends the patch upstream.)

## What shipped

`internal/diskfmt.FormatFAT32` mirrors go-diskfs's layout arithmetic and refuses
an oversized device before writing a byte:

> `/dev/sda is 476.84 GiB, and GoSD cannot create a FAT32 volume larger than
> 256.06 GiB: GoSD's FAT32 formatter counts the sectors in each file allocation
> table in 16 bits, so a larger volume would be laid out with FATs far too small
> for it and silently corrupted; format this device as exFAT instead
> (disk.Options{Filesystem: disk.ExFAT}), which has no such limit and suits media
> this large`

Boundary tests are compute-only (`checkFAT32Size` at the limit, one sector over,
and the sizes real drives come in), plus one behavioural test that a 512 GB
sparse device is refused and left blank. Docs now carry the ceiling next to
FAT32's 4 GiB per-file one: `disk`'s package doc, `docs/runtime.md`'s "FAT32 or
exFAT", COMPATIBILITY.md's exFAT footnote.

## Other callers audited

- **`emmc`** reaches `FormatFAT32` through the same `internal/blockmount` path,
  so it was exposed in principle — but only in principle: eMMC parts top out
  around 256 GB (238 GiB), under the limit, and GoSD's boards carry 8-64 GB. It
  is guarded anyway. (The error's exFAT suggestion does not apply
  to `emmc`, which is FAT32-only by design — but no real eMMC can reach it.)
- **`cmd/gosd-init/internal/dataexpand`** already caps a grown partition at
  256 GiB (`maxPartitionBytes`) with a logged notice, so it cannot reach the
  defect.
- **`internal/image`** (build host, not device) formats the image's GOSD-DATA
  partition through the same go-diskfs call with the developer's `--data-size`,
  and was **not** guarded by this change — a `gosd build` CLI behaviour decision
  rather than this bean's device-side scope, left for JP. His answer was to
  refuse: `cmd/gosd`'s `parseDataSize` now rejects anything past the same limit
  at flag validation, before a byte is compiled or written (bean `gosd-mt53`).
  `internal/image` itself remains structurally able to write an oversized
  volume, but nothing can reach it that way: `gosd build` and `gosd run` are its
  only callers, and both size the partition through `parseDataSize`.

## Upstream fix — prepared, NOT sent

No fork, branch or PR exists; sending this to `diskfs/go-diskfs` is JP's call.
The patch applies to v1.9.3 and to `master` (the code is identical today,
checked 2026-07-30).

```diff
--- a/filesystem/fat32/fat32.go
+++ b/filesystem/fat32/fat32.go
@@ -128,11 +128,7 @@ func Create(b backend.Storage, size, start, blocksize int64, volumeLabel string,
 	totalSectors := uint32(size / blocksize)
-	// Closed-form equivalent of the dosfstools mkfs.fat sectors-per-FAT search:
-	// smallest X such that (reserved + 2X + clusters*SPC) == totalSectors and
-	// X * (bytesPerSector/4) >= clusters + 2.
-	fatEntryDenom := uint32(blocksize)*uint32(sectorsPerCluster) + 8
-	sectorsPerFat := uint16((4*(totalSectors-uint32(reservedSectors)) + fatEntryDenom - 1) / fatEntryDenom)
+	sectorsPerFat := sectorsPerFatFor(totalSectors, reservedSectors, blocksize, sectorsPerCluster)
 
 	// The layout must yield at least one cluster and leave at least 32 KiB
 	// of data area beyond the reserved sectors and FATs (matches mkfs.fat checks).
@@ -184,7 +180,7 @@ func Create(b backend.Storage, size, start, blocksize int64, volumeLabel string,
 		driveNumber:           128,
-		sectorsPerFat:         uint32(sectorsPerFat),
+		sectorsPerFat:         sectorsPerFat,
 	}
 
@@ -194,7 +190,7 @@ func Create(b backend.Storage, size, start, blocksize int64, volumeLabel string,
 	fatPrimaryStart := uint64(reservedSectors) * uint64(blocksize)
-	fatSize := uint32(sectorsPerFat) * uint32(blocksize)
+	fatSize := sectorsPerFat * uint32(blocksize)
 	fatSecondaryStart := fatPrimaryStart + uint64(fatSize)
 
+// sectorsPerFatFor is the closed-form equivalent of the dosfstools mkfs.fat
+// sectors-per-FAT search: the smallest X such that
+// (reserved + 2X + clusters*SPC) == totalSectors and
+// X * (bytesPerSector/4) >= clusters + 2.
+//
+// It is computed in 64 bits and returned as a uint32, the width of the on-disk
+// FATSz32 field. A FAT32 volume may span just under 2^32 sectors
+// (Fat32MaxSize), so the numerator overflows a uint32 near 512 GiB and the
+// count itself passes 65535 near 256 GiB; either overflow lays FATs out far
+// too small for the volume's clusters.
+func sectorsPerFatFor(totalSectors uint32, reservedSectors uint16, blocksize int64, sectorsPerCluster uint8) uint32 {
+	fatEntryDenom := uint64(blocksize)*uint64(sectorsPerCluster) + 8
+	return uint32((4*(uint64(totalSectors)-uint64(reservedSectors)) + fatEntryDenom - 1) / fatEntryDenom)
+}
```

Test to go with it (`filesystem/fat32/sectorsperfat_internal_test.go`), which
needs no large allocation:

```go
package fat32

import "testing"

// Large volumes are what the width of this calculation is about: a FAT32
// volume may span just under 2^32 sectors, at which point both the count and
// the intermediate 4*totalSectors exceed the uint16 and uint32 the calculation
// used to be done in. Expected values are ceil(4*(T-32)/32776), computed
// exactly.
func TestSectorsPerFatIsNotTruncatedOnLargeVolumes(t *testing.T) {
	const (
		blocksize         = int64(512)
		reservedSectors   = uint16(32)
		sectorsPerCluster = uint8(64) // 32 KiB clusters: what Create picks past 32 GB
	)
	for _, tt := range []struct {
		name         string
		totalSectors uint32
		want         uint32
	}{
		{"64 GiB", 134217728, 16380},
		{"256 GiB", 536870912, 65520},
		{"largest count a uint16 could hold", 536993822, 65535},
		{"one sector more", 536993823, 65536},
		{"512 GB SSD", 1000000000, 122041},
		{"512 GiB", 1073741824, 131041},
		{"1 TB drive", 1953525168, 238410},
		{"Fat32MaxSize", 4294441600, 524096},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := sectorsPerFatFor(tt.totalSectors, reservedSectors, blocksize, sectorsPerCluster); got != tt.want {
				t.Errorf("sectorsPerFatFor(%d) = %d, want %d", tt.totalSectors, got, tt.want)
			}
		})
	}
}
```

Description to paste into the upstream PR:

> **fat32: compute sectors-per-FAT in 64 bits (FATSz32 is a 32-bit field)**
>
> `fat32.Create` narrows its sectors-per-FAT count to a `uint16`:
>
> ```go
> sectorsPerFat := uint16((4*(totalSectors-uint32(reservedSectors)) + fatEntryDenom - 1) / fatEntryDenom)
> ```
>
> `FATSz32` is a 32-bit field on disk, and `Fat32MaxSize` allows volumes up to
> 2 TiB, so this truncates well inside the supported range. With the 32 KiB
> clusters `Create` picks past 32 GB, one FAT needs
> `ceil(4*(totalSectors-32) / (512*64+8))` sectors, which exceeds 65535 once the
> volume passes 536,993,822 sectors (256.06 GiB). Above that the volume is laid
> out with FATs far too small to address its clusters and written with no error:
> a 512 GB SSD records 56505 sectors per FAT where it needs 122041.
>
> The same expression is evaluated in `uint32` arithmetic, so
> `4*(totalSectors-32) + fatEntryDenom - 1` also overflows near 512 GiB; there
> the count becomes 0 and `Create` panics with
> `index out of range [2] with length 1` while filling the cluster table.
>
> This computes the value in 64 bits and keeps it in a `uint32`, matching the
> on-disk field, with a table test at the boundary (no large allocations).
> Nothing below 256 GiB changes.

## Follow-up

- [ ] Once a `go-diskfs` release carries the 64-bit fix, bump the pin and drop
      **all three** caps together — `internal/diskfmt`'s `maxFAT32Bytes` guard,
      `cmd/gosd-init/internal/dataexpand`'s `maxPartitionBytes` (256 GiB), and
      `cmd/gosd`'s `--data-size` refusal. They exist for one reason and removing
      any one alone is misleading. Before lifting them, check memory:
      `fat32.Create` holds the whole FAT in RAM (`make([]uint32, fatSize/4)`),
      ~120 MB for a 1 TB volume, on boards with 512 MB. This route, and the
      alternatives to it, are laid out in bean `gosd-mt53`.
- [x] Decide whether `gosd build --data-size` should refuse (or cap) sizes above
      the same limit — decided: **refuse**, at flag validation in `cmd/gosd`'s
      `parseDataSize`, naming the maximum in both GiB and bytes and linking
      `docs/runtime.md#how-big-the-data-partition-can-be`. Capping silently
      would hand a developer a smaller partition than they asked for with no say
      in it, and a fixed `--data-size` that large implies an `.img` file that
      large anyway, so there is no size worth quietly rounding down. The reason
      clause is now `diskfmt.FAT32SizeLimitReason`, shared with the runtime
      refusal above so the two tell one story (bean `gosd-mt53`).

## Cross-reference

gosd-qvjs added a regression test (TestFAT32MirroredArithmeticMatchesGoDiskfsRealOutput, internal/diskfmt/fat32limit_test.go) that pins go-diskfs v1.9.3's real sectors-per-FAT/sectors-per-cluster behavior against this package's mirrored formulas. Landing this bean's upstream fix removes the need for that mirror (and the test) entirely — extra motivation beyond upstream cleanliness.
