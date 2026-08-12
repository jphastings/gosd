---
# gosd-0s0m
title: 'Spike: pure-Go on-device FAT32 format of a block device (go-diskfs)'
status: completed
type: task
priority: normal
created_at: 2026-07-10T07:46:58Z
updated_at: 2026-07-10T08:35:19Z
parent: gosd-jge2
blocking:
    - gosd-tdcc
---

Prove (or disprove) that we can format the onboard eMMC on-device, in pure Go
(`CGO_ENABLED=0`, no root beyond write access to the device node, no external
`mkfs`/`fdisk`), so that a blank soldered eMMC can be made usable. This is the
load-bearing unknown behind option A of [[gosd-899s]]; it unblocks (and
re-scopes) [[gosd-tdcc]] from mount-only to format-or-mount.

## Goal / question
`go-diskfs` v1.9.3 is already a dependency and already creates MBR + FAT32
*image files* (`internal/image/image.go` via `diskfs.Create` → `Partition` →
`CreateFilesystem`). Can the same library instead target a **real Linux block
device node** (`/dev/mmcblk0` for the whole device, or `/dev/mmcblk0p1` for a
partition) and lay down a FAT32 filesystem there?

The single riskiest sub-question: **disk-size detection.** `diskfs.Create`
takes an explicit size; `diskfs.Open` of a block-device special file may see a
`Stat` size of 0. Determine how go-diskfs sizes an existing block device, and
if it can't, whether we can supply the size ourselves (ioctl `BLKGETSIZE64` /
seek-to-end) and drive the format anyway.

## What to establish
- [x] How `go-diskfs` opens/sizes an existing device vs an image file; whether
      a supported path exists to format a real `/dev/mmcblkN` (quote the size
      path, note function signatures + file:line).
- [x] Whether whole-device (partitionless) FAT is possible, or an MBR partition
      is required first; pick the simpler viable shape for eMMC.
- [x] Minimal proof-of-concept that runs on our dev hosts without root:
      format a backing file that stands in for the device (and, if a size
      override is needed, exercise that path), then verify the result is a
      valid FAT32 filesystem by reading it back with go-diskfs (and, where
      available, `fsck.fat`/mount). Keep it throwaway or behind a clearly
      spike-scoped package.
- [x] Confirm the FAT32 + MBR code paths are pure Go (cross-compile the PoC
      for `GOOS=linux GOARCH=arm64` and `GOARCH=arm GOARM=6`, `CGO_ENABLED=0`).
- [x] FAT32 size-floor gotcha: some libraries force FAT16 below ~32MiB — note
      go-diskfs's behaviour and any minimum eMMC size implication.

## Deliverable
Findings recorded here, plus a clear recommendation: is on-device pure-Go
FAT32 formatting of the eMMC viable? If yes, sketch the API the format-or-mount
library needs (e.g. `device.FormatEMMC` / a format-if-blank branch inside
`MountEMMC`) and note the size-detection approach. If no, say what blocks it
and fall back to option B/D in [[gosd-899s]].

## Notes / constraints
- Cannot fully validate against real eMMC without hardware — the PoC proves the
  library mechanics; a hardware check on Radxa Zero 3E / NanoPi Zero2 stays a
  follow-up (mirrors the hardware-test tail on [[gosd-xelb]]).
- Kernel is VFAT-only on both Rockchip boards, so FAT is the only on-device
  target that can also be mounted afterwards.

## Findings (2026-07-10)

**Verdict: viable.** On-device pure-Go FAT32 formatting of the eMMC works with
the `go-diskfs` v1.9.3 we already depend on — no new deps, no cgo, no external
`mkfs`, no root beyond write access to the node. Option A of [[gosd-899s]] is a
go.

- **Block-device sizing is automatic.** `diskfs.Open` detects a block-device
  special file and reads its real size via `ioctl(BLKGETSIZE64)` (plus
  `BLKSSZGET`/`BLKPBSZGET` for sector sizes) in `diskfs_linux.go`; regular files
  fall back to `Stat` size. So we do **not** need our own ioctl/seek — no size
  override is required (and none exists). Darwin uses the `DKIOCGET*`
  equivalents, so the PoC test even sizes correctly on macOS.
- **Whole-device, partitionless FAT is the right shape.** `CreateFilesystem`
  with `Partition: 0` formats the entire device with no MBR. This also sidesteps
  the `BLKRRPART` partition-table reread — the one step that needs privileges on
  real hardware. So: open + format, nothing else.
- **Open mode matters on hardware.** diskfs' default open is
  `ReadWriteExclusive` (`O_EXCL`), which fails when the kernel already holds the
  block device. Use `diskfs.WithOpenMode(diskfs.ReadWrite)`. `diskfs.Create` is
  the wrong call for an existing node (it's `O_CREATE|O_EXCL`, needs a
  non-existent path); use `diskfs.Open`.
- **Pure Go confirmed.** The FAT32 + MBR code paths import only
  `golang.org/x/sys/unix`; no `import "C"`, `os/exec`, or `mount`. The PoC
  cross-compiles `CGO_ENABLED=0` for `linux/arm64` and `linux/arm GOARM=6`.
- **FAT32 size floor is a non-issue.** go-diskfs does not force FAT16 below any
  ~32 MB threshold (unlike some libs); FAT32 works from ~tens of KiB upward
  (reserved-region floor 16 KiB, data-area floor 32 KiB), far below any real
  eMMC. We pass `FSType` explicitly, so there's no surprise auto-selection.

**PoC:** `internal/emmcfmt` — `FormatFAT32(devicePath, volumeLabel)` (open
read-write, whole-device FAT32) with a behavioural test that formats a sparse
backing file, then proves a file survives a write → reopen → read round-trip and
that the volume is FAT32 with the expected label. Runs on macOS; passes all
quality gates.

**Retarget delta from `internal/image`:** the only substantive change from the
image-file path is the open call (`diskfs.Create(path,size,…)` →
`diskfs.Open(path, WithOpenMode(ReadWrite), WithSectorSize(512))`) plus dropping
the partition table (`Partition: 0`). Everything downstream is identical.

**Follow-ups for the format-or-mount library ([[gosd-tdcc]]):**
- "Format only if blank" logic: try to read an existing FAT filesystem first;
  only format when there's nothing mountable (avoid nuking user data).
- Whether to expose formatting as an explicit `device.FormatEMMC` vs an
  opt-in `format-if-blank` flag on the mount call (destructive — must be
  deliberate).
- **Real-hardware validation** on Radxa Zero 3E + NanoPi Zero2: format the
  actual `/dev/mmcblk0`, confirm `O_EXCL`/`ReadWrite` behaviour and that the
  kernel then mounts it. The PoC exercises everything except the `BLKGETSIZE64`
  ioctl, which needs a real device; this stays a hardware follow-up (mirrors the
  torture-test tail on [[gosd-xelb]]).

## Summary of Changes

Spiked pure-Go on-device eMMC formatting and confirmed it's viable with the
existing `go-diskfs` dependency. Added `internal/emmcfmt` (a deliberately
minimal `FormatFAT32` + round-trip behavioural test) as the proof; it will be
folded into the format-or-mount work under [[gosd-tdcc]], which is now unblocked
and should be re-scoped from mount-only to format-if-blank + mount.
