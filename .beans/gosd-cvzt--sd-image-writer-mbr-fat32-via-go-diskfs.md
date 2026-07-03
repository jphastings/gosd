---
# gosd-cvzt
title: SD image writer (MBR + FAT32) via go-diskfs
status: todo
type: task
created_at: 2026-07-02T20:53:02Z
updated_at: 2026-07-02T20:53:02Z
parent: gosd-vi0n
blocked_by:
    - gosd-56xt
---

Write the flashable .img in `internal/image`, pure Go, no root, using github.com/diskfs/go-diskfs.

Locked layout: MBR partition table. Partition 1: FAT32, partition type 0x0C, label GOSD-BOOT, starts at exactly 16MiB (LBA 32768 at 512b sectors), size 256MiB. Total image size 272MiB + MBR slack. The 512B–16MiB gap stays unpartitioned (Rockchip bootloader lands there — see the Radxa embed task).

API (locked):
`image.Write(path string, spec Spec) error` where Spec has: `BootFiles map[string]io.Reader` (paths inside the FAT partition, subdirectories allowed) and `RawWrites []RawWrite{OffsetBytes int64; Content io.Reader}` written into the unpartitioned gap after partitioning.

- [ ] Create sparse-friendly image file, MBR, FAT32 partition via go-diskfs
- [ ] Populate FAT from BootFiles (create subdirectories as needed)
- [ ] Apply RawWrites, erroring if any write would overlap partition 1 or the MBR
- [ ] Unit test: write an image with nested files + a RawWrite at LBA 64, read it back with go-diskfs, assert file contents and raw bytes; assert first partition offset is 16MiB

## Acceptance
`go test ./internal/image` passes; produced image is recognized by `fdisk -l` (document the manual check, do not run in CI).
