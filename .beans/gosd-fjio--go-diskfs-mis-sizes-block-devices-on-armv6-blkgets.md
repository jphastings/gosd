---
# gosd-fjio
title: go-diskfs mis-sizes block devices on armv6 (BLKGETSIZE64 into a 32-bit int)
status: todo
type: bug
created_at: 2026-07-29T17:54:56Z
updated_at: 2026-07-29T17:54:56Z
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
