---
# gosd-fjio
title: go-diskfs mis-sizes block devices on armv6 (BLKGETSIZE64 into a 32-bit int)
status: completed
type: bug
priority: normal
created_at: 2026-07-29T17:54:56Z
updated_at: 2026-07-30T20:24:20Z
---

Spotted during the exFAT work (bean gosd-1ici, 2026-07-29) and verified against the pinned dependency: `go-diskfs@v1.9.3/diskfs_linux.go:12` sizes a block device with

    blockDeviceSize, err := unix.IoctlGetInt(int(f.Fd()), unix.BLKGETSIZE64)

`unix.IoctlGetInt` passes a pointer to a Go `int`. On `GOARCH=arm` (pi-zero-w — our only 32-bit board) that is a **4-byte** destination, while the kernel's BLKGETSIZE64 handler writes a **u64 unconditionally** (`put_u64(argp, bdev_nr_bytes(bdev))`), so 8 bytes land in a 4-byte stack variable.

Two consequences on armv6, both latent today:
1. **Memory corruption**: 4 bytes written past the destination. Little-endian means the high half lands in adjacent stack memory — harmless-looking for devices under 4 GiB (high half is zero) and undefined generally.
2. **Wrong size**: any device >= 4 GiB (i.e. every modern SD card, USB stick or SSD) reports a truncated size, so a filesystem gets laid out for a fraction of the disk.

Reachability: `emmc` never hits it on pi-zero-w (no onboard eMMC → ErrNoEMMC first), but the new `disk` package does the moment a USB drive is attached to a Zero W — which is exactly the bench scenario in gosd-yggd's checklist. arm64 boards are unaffected (8-byte int).

Fix options, in preference order:
- **(a) Don't ask go-diskfs for the size.** Read `/sys/block/<dev>/size` (512-byte sectors, always a decimal string, no ioctl, no arch assumptions) in our own code and hand go-diskfs an explicit size. Pure Go, testable from a fake sysfs, fixes both consequences, no upstream dependency. Check what `diskfs.Open`/`OpenWithMode` accept for an explicit size.
- (b) Upstream PR to go-diskfs using a `uint64` + `ioctlPtr` (correct for every arch), then wait for a release and bump. Worth doing regardless as good citizenship, but not our critical path.

Also confirm whether `BLKSSZGET`/`BLKPBSZGET` on the adjacent lines are similarly affected (those genuinely return `int`, so they are probably fine — verify rather than assume).

Bench angle: the fix is verifiable on hardware by formatting a >4 GiB USB stick on a Zero W and checking the resulting filesystem spans the whole device.

## Summary of Changes

Fixed via option (a): `internal/diskfmt` no longer uses `diskfs.Open` at all. A new `openDisk` helper opens the device node itself and sizes it with `lseek(SEEK_END)` — 64-bit on every Linux arch Go supports, valid for block devices and regular files alike — then constructs `disk.Disk` directly (its fields are exported; `CreateFilesystem`/`GetFilesystem` with Partition 0 only consume Backend/Size/LogicalBlocksize). All three sizing paths (`Inspect`'s FAT probe, `FormatFAT32`, and `FormatExFAT`, which shared the same broken open) now go through it. Sector size is fixed at 512, which every target medium presents and which `FormatExFAT` already hardcoded, so the `BLKSSZGET`/`BLKPBSZGET` question is moot — those ioctls are no longer called either.

`TestFormatFAT32SpansADevicePast4GiB` pins the consequence-2 behavior (a 5 GiB device's FAT32 geometry spans the whole device); the consequence-1 corruption argument is structural (the ioctl is gone). Not done here: the upstream go-diskfs PR (option b) and the Zero W bench verification with a >4 GiB USB stick — both still worthwhile, neither on the critical path.
