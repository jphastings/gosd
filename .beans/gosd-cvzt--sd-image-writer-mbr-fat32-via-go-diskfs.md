---
# gosd-cvzt
title: SD image writer (MBR + FAT32) via go-diskfs
status: completed
type: task
priority: normal
created_at: 2026-07-02T20:53:02Z
updated_at: 2026-07-03T17:53:03Z
parent: gosd-vi0n
blocked_by:
    - gosd-56xt
---

Write the flashable .img in `internal/image`, pure Go, no root, using github.com/diskfs/go-diskfs.

Locked layout: MBR partition table. Partition 1: FAT32, partition type 0x0C, label GOSD-BOOT, starts at exactly 16MiB (LBA 32768 at 512b sectors), size 256MiB. Total image size 272MiB + MBR slack. The 512B–16MiB gap stays unpartitioned (Rockchip bootloader lands there — see the Radxa embed task).

API (locked):
`image.Write(path string, spec Spec) error` where Spec has: `BootFiles map[string]io.Reader` (paths inside the FAT partition, subdirectories allowed) and `RawWrites []RawWrite{OffsetBytes int64; Content io.Reader}` written into the unpartitioned gap after partitioning.

- [x] Create sparse-friendly image file, MBR, FAT32 partition via go-diskfs
- [x] Populate FAT from BootFiles (create subdirectories as needed)
- [x] Apply RawWrites, erroring if any write would overlap partition 1 or the MBR
- [x] Unit test: write an image with nested files + a RawWrite at LBA 64, read it back with go-diskfs, assert file contents and raw bytes; assert first partition offset is 16MiB

## Acceptance
`go test ./internal/image` passes; produced image is recognized by `fdisk -l` (document the manual check, do not run in CI).

## Summary of Changes

Implemented `internal/image.Write(imgPath string, spec Spec) error` per the locked API: `Spec{BootFiles map[string]io.Reader, RawWrites []RawWrite}`. Uses github.com/diskfs/go-diskfs to create a 272MiB sparse-friendly image file (via os.Truncate), write an MBR with a single partition (type 0x0C/Fat32LBA, start LBA 32768 = 16MiB, size 524288 sectors = 256MiB), format it FAT32 with label GOSD-BOOT, populate it from BootFiles (creating parent directories via fs.Mkdir as needed, sorted for determinism), and apply RawWrites directly to the backend after partitioning.

RawWrite bounds checking (checkRawWriteBounds/rangesOverlap in internal/image/image.go) reads each write's content fully first (to know its length), then rejects — wrapping the exported `ErrRawWriteOverlap` sentinel — any write whose byte range intersects the MBR (bytes 0-512) or partition 1 (bytes 16MiB-272MiB), as well as any write that would run past the end of the image. Covered by three dedicated tests (overlapping the MBR, overlapping partition 1, and straddling the gap boundary into partition 1) plus a round-trip test that reads the image back with go-diskfs and asserts nested file contents, raw bytes, and the 16MiB partition-1 offset.

The pre-existing stub `Assembler`/`Spec` (used by `cmd/gosd build`'s pipeline) was renamed to `AssembleSpec` to free up the `Spec` name for the bean's locked low-level API; `NotImplemented` still returns a clear not-wired error, since feeding initramfs + board boot files into `image.Write` is gosd-3zrc's job, not this bean's.

go-diskfs quirk found and worked around: `disk.Disk.GetPartition`/`CreateFilesystem` match partitions by their `Index` field, which `mbr.Partition` literals do **not** get for free — it must be set explicitly (`Index: 1`) or "partition 1 not found" errors even though the partition was written correctly. No FAT32 minimum-size issue was hit at 256MiB (go-diskfs's FAT32 minimum is ~16KB of reserved sectors plus 32KiB of data area, far below our size).

Manual check performed as documented in the Acceptance section: wrote a sample image and ran macOS's `fdisk` on it (not run in CI) — it reports partition 1 as type `0C` ("Win95 FAT32L"), start sector 32768, size 524288 sectors, matching the locked layout exactly.
