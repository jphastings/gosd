---
# gosd-58p6
title: Bench-verify an ext4 /data on pi-zero-w and pi-3b
status: todo
type: task
priority: normal
created_at: 2026-08-10T10:14:46Z
updated_at: 2026-08-10T12:45:08Z
---

PR #248 (bean gosd-ssth) enabled `gosd build --data-filesystem=ext4` for all three Pi boards, but only **pi-zero-2w** was actually bench-booted (bean gosd-7bwv). The other two ride released-kernel evidence alone: their v0.10.0 kernel.configs carry CONFIG_EXT4_FS=y and their kernel binaries contain the driver including ext4_resize_fs. COMPATIBILITY.md's [^pi-data-ext4] footnote and the UNRELEASED.md release note both record this split honestly, so this bean closes the gap rather than a known defect.

**pi-zero-w is the one that matters.** It is the fleet's only 32-bit board (GOARCH=arm GOARM=6) and would be the first board ever to run ext4 on armv6 — every other ext4-capable board is arm64. The code was written with this in mind (blockmount.blockDeviceSizeBytes reads BLKGETSIZE64 into an explicit uint64 rather than the word-sized unix.Ioctl* helpers, citing bean gosd-fjio for the truncation that would otherwise bite on 32-bit ARM; the EXT4_IOC_RESIZE_FS number is computed from the generic _IOC layout, identical on arm and arm64; targetBlocks is a uint64 passed by pointer) but none of that has been executed on a 32-bit board. Also worth confirming the golden's 64bit/meta_bg/metadata_csum_seed feature set mounts and grows on a 32-bit kernel — its 6.18.37 build does contain the meta_bg resize paths (add_new_gdb_meta_bg, ext4_convert_meta_bg).

pi-3b is much lower risk: same arm64 kernel pin as the Zero 2W, so this is really a spot-check.

Method that worked on the Zero 2W and needs no serial console (the bench Pi's is dead, bean gosd-y3vm) and no ext4 support on the host: flash an ext4 --data-size=expand image containing only partition 1, boot, then read the card's MBR with `diskutil list`. dataexpand's expand path writes the MBR LAST, only after format -> sync -> mount -> grow -> marker -> sync all succeed, so a full-size partition 2 appearing is proof the whole chain ran. For the durability half, provision WiFi and curl the app's boots=N across an `sdwire power off` — full recipe in gosd-7bwv.



## pi-zero-w VERIFIED on hardware (2026-08-10) — the 32-bit question is answered

Done on the bench Pi Zero W (boardrev 0x9000c1) with a working serial console, so this was watched live rather than inferred from the partition table. Image: examples/hello, --data-filesystem=ext4 --data-size=expand, pi-zero-w (GOARCH=arm GOARM=6).

First boot:
    [gosd] data partition filesystem: ext4
    [gosd] formatting /dev/mmcblk1p2 as ext4 labelled hello-data (14.6GiB) — one-time first-boot setup
    [gosd] data partition created, filling the card
    [gosd] data partition mounted read-write at /data
    gosd hello, host=pi-ext4w board=pi-zero-w boots=1

So on armv6: the golden was written, EXT4_IOC_RESIZE_FS grew it from 512MiB to 14.6GiB, and it mounted read-write. That exercises exactly the 32-bit hazards this bean was filed for — blockmount.blockDeviceSizeBytes's uint64 BLKGETSIZE64 read (bean gosd-fjio) and the generic _IOC ioctl encoding — on the only 32-bit board in the fleet.

Then an abrupt mains cut (sdwire power off, no clean shutdown) with the counter write seconds old, and on the next boot:
    [gosd] data partition already present on /dev/mmcblk1p2
    [gosd] data partition mounted read-write at /data
    gosd hello, host=pi-ext4w board=pi-zero-w boots=2

boots=2 proves the fsync'd write reached the card through ext4's journal and replayed cleanly on mount, and 'already present' proves adoption rather than reformat.

REMAINING: pi-3b only. It shares the Zero 2W's arm64 kernel pin and is the lowest-risk of the three, but it has still never run an ext4 /data on hardware. Keeping this bean open for that alone.
