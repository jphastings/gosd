---
# gosd-fija
title: 'image/boards: idbloader.img overlapping u-boot.itb''s offset is never checked — silent bootloader corruption if idbloader grows'
status: completed
type: bug
priority: high
created_at: 2026-07-31T07:52:39Z
updated_at: 2026-07-31T07:52:39Z
---

Found by review sweep `gosd-fuxs` (build pipeline area), verified.

All three Rockchip boards' `RawWrites` (radxazero3e/board.go:135-152,
nanopizero2, rock4se) assert only that u-boot.itb ends before the 16MiB
boot-partition start. Nothing asserts idbloader.img (written at 32KiB)
ends before `ubootOffsetBytes` (8MiB), and
`internal/image.checkRawWriteBounds` (image.go:283-311) checks each write
against the MBR/partitions individually — it has no knowledge of sibling
writes, so two raw writes overlapping *each other* are never detected.
Design bean `gosd-gbsz` recorded the false assumption: "the image writer
already enforces non-overlap".

**Failure scenario:** a future TPL/SPL config grows idbloader.img past
8,355,840 bytes. Both bounds checks pass; u-boot.itb is written at 8MiB
over idbloader's tail. `gosd build` succeeds, producing a structurally
valid image with a corrupted bootloader — no error, no diagnostic, fails
only on hardware.

**Fix:** make `applyRawWrites` check all writes pairwise with the existing
`rangesOverlap` helper (covers every future board), and/or mirror the
u-boot assertion for idbloader in each board. Add a board test seeding an
oversized idbloader.img.

## Summary of Changes
`internal/image.applyRawWrites` (image.go) now checks every `RawWrite` pair
for overlap, not just each write against the MBR/partitions individually.
RawWrite carries an `io.Reader`, not a length, so the function was
restructured into two passes over the same single read of each write's
content (no double-reading): pass one reads each write's bytes into a new
`resolvedRawWrite{offsetBytes, data}` and runs the existing
`checkRawWriteBounds`; pass two, once every write's length is known, runs a
new `checkRawWritesDontOverlap` (O(n²) pairwise `rangesOverlap`, n is the
handful of writes one board contributes) before anything is written to
disk. A sibling-overlap failure now surfaces before any bytes are written,
rather than partially writing earlier siblings first. The overlap error is
wrapped in the existing `ErrRawWriteOverlap` and names both writes' offsets,
lengths, and end bytes, per the repo's actionable-error convention;
`ErrRawWriteOverlap`'s doc comment and message were broadened from
"MBR or the boot partition" to "MBR, a partition, or another raw write".
This is the image-level fix the bean preferred over duplicating a
per-board idbloader/u-boot assertion, since it covers every board's raw
writes (current and future) from one place.

Tests: `internal/image/image_test.go` adds
`TestWriteRejectsTwoRawWritesThatOverlapEachOther`, two synthetic raw
writes shaped like a Rockchip board's idbloader.img growing into
u-boot.itb's offset, asserting `errors.Is(err, image.ErrRawWriteOverlap)`
and that the error names the offending offsets/lengths.
`internal/boards/radxazero3e/board_test.go` adds
`TestBuildFailsWhenIdbloaderGrowsIntoUboot`, seeding a fake
`--artifacts-dir` with an oversized `idbloader.img` (sized to run exactly
into `u-boot.itb`'s locked offset) and asserting `image.Write` with that
board's real `RawWrites()` output fails with `ErrRawWriteOverlap` instead
of silently producing a corrupted image - the failure mode the bean
describes never reaches `checkRawWriteBounds` cleanly since each
individual write still lands inside the unpartitioned gap.

Quality gates (`go test ./...`, `go vet ./...`, `gofmt -l .`,
`golangci-lint run ./...`, `GOOS=linux golangci-lint run ./...`) all pass.
