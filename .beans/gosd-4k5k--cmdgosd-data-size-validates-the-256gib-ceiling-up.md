---
# gosd-4k5k
title: 'cmd/gosd: --data-size validates the 256GiB ceiling up front but not the sub-sector floor — a full build runs before failing'
status: completed
type: task
priority: low
created_at: 2026-07-31T07:54:53Z
updated_at: 2026-08-20T06:27:17Z
---

Found by review sweep `gosd-fuxs` (build pipeline area), verified.

parseDataSize (cmd/gosd/build.go:282-310) rejects negatives, overflow, and
> MaxFAT32Bytes — but 0 < size < 512 passes and only fails deep in
computeLayout (image.go:121-124) after cross-compiling the app and
gosd-init and fetching artifacts for every board. The ceiling check's own
doc says it exists to refuse "before any image bytes exist".

**Fix:** symmetric floor check in parseDataSize (reject 0 < size < 512
with the same actionable style). Arguably reject anything below a useful
minimum (one FAT32 cluster + overhead) with a message naming it.

## Summary of Changes

`cmd/gosd/build.go`'s `parseDataSize` now rejects a `--data-size` that rounds down to fewer than one 512-byte sector as soon as the flag is parsed (checked right after `parseSizeBytes`, before the ext4/FAT32-specific branches), instead of surviving a full cross-compile and artifact fetch for every board only to fail deep inside `image.computeLayout`. This matches the existing ceiling check's and the ext4 golden-image floor's own "fail before any image bytes exist" contract. `--data-size=0` remains valid (disables the data partition for FAT32; still refused for ext4, for its own pre-existing, unrelated reason).

`internal/image/image.go` exports `SectorSizeBytes` (mirroring the existing unexported `sectorSizeBytes`, same pattern as the already-exported `DefaultBootPartitionSizeBytes`) so `cmd/gosd` can name the same 512-byte boundary `computeLayout` enforces internally, rather than duplicating the number.

`docs/runtime.md`'s "How big the data partition can be" section — the page the ceiling refusal links to — now mentions the sub-sector floor alongside the existing ceiling and ext4-floor explanation.

Added `cmd/gosd/build_test.go` tests: `TestParseDataSizeRefusesSubSectorSizes` (0/1/511/512-byte boundary cases for both FAT32 and ext4) and `TestParseDataSizeSubSectorRefusalNamesTheSector` (asserts the refusal names the byte count, the sector size, and the likely missing-unit-suffix mistake).
