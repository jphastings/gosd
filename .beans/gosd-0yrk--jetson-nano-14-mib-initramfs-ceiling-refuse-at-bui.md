---
# gosd-0yrk
title: 'Jetson Nano: 14 MiB initramfs ceiling — refuse at build time or lift via boot.scr'
status: completed
type: task
created_at: 2026-11-01T11:53:20Z
updated_at: 2026-11-01T11:53:20Z
parent: gosd-72je
blocking:
    - gosd-uw32
---

NVIDIA's stock U-Boot loads the initrd at ramdisk_addr_r=0x83200000 and the kernel at kernel_addr_r=0x84000000. That leaves 14 MiB (14,680,064 B) for the initramfs, whose tail is otherwise silently overwritten: "Initramfs unpacking failed", then a panic=10 reboot loop (bean gosd-cxio's findings, from include/configs/tegra210-common.h:53-60 at L4T R32.6.1). hello fits at ~10.9 MB; hello --ingress cloudflared is 28.4 MB. CONFIG_USE_PREBOOT is off, so u-boot-preboot.scr never runs.

Two options:
- (a) Refuse at build time. Board.BootFiles only ever sees a placeholder, since the real initramfs is built after it in internal/pipeline. So this needs a small pipeline-side check, e.g. an optional board capability consulted after initramfs.Build. It keeps the ceiling but turns the silent bench failure into an actionable error.
- (b) Lift it. Ship boot.scr (a legacy uImage script; the header is generatable in a few lines of Go) that sets ramdisk_addr_r above pxefile_addr_r, then sysboot's our extlinux.conf from a path distro boot doesn't scan itself. Distro boot tries extlinux before scripts, so /extlinux/extlinux.conf must move.

Settle the bench's printenv on R32.7.6 first (gosd-9o0m): the addresses here are unconfirmed for the final firmware. Do (a) regardless before activation; do (b) if app sizes justify it.

## Summary of Changes

Option (a) done: an optional `boards.InitramfsLimiter` interface, implemented by `jetsonnano` (14,680,064 bytes), is checked in `internal/pipeline` right after the real initramfs is built. An over-limit build is refused with the board, the measured and permitted sizes in MiB, and what to try.

Option (b), lifting the ceiling via `boot.scr`, is NOT done. It waits for the bench's U-Boot `printenv` on the final R32.7.6 firmware (bean gosd-9o0m).
