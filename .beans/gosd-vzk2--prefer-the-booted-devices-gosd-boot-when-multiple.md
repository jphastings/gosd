---
# gosd-vzk2
title: Prefer the booted device's GOSD-BOOT when multiple GoSD boot partitions exist
status: todo
type: task
created_at: 2026-07-24T07:49:39Z
updated_at: 2026-07-24T07:49:39Z
---

gosd-pcwl fixed gosd-init's boot-partition probe to reject candidates that
merely mount as valid FAT but aren't a GoSD boot partition at all (checks for
gosd.toml at the root; unmounts and keeps probing if absent). That closes the
"vendor image / arbitrary FAT" case, but it cannot distinguish between two
*actual* GoSD boot partitions: an eMMC that itself carries a stale,
previously-flashed GoSD image also has gosd.toml at its root, passes the
sentinel check, and still wins simply by sorting first in device-name order
(mmcblk0 before mmcblk1). The probe has no way to know which physical device
the SoC's boot ROM/U-Boot actually booted from — it only knows device names.

Repro scenario: NanoPi Zero2 (or any board with both eMMC and SD boot media)
with a GoSD image previously flashed to the eMMC, then a *different*/updated
GoSD image freshly flashed to the SD card. gosd-init boots from the SD's
kernel/initramfs (that part is fine — the boot ROM's own media selection
picked the SD), but the boot-partition probe still mounts the eMMC's
GOSD-BOOT as /boot: stale gosd.toml, stale app config, from an image the user
didn't intend to use this boot.

Design direction (not locked, needs its own investigation): have U-Boot pass
the device it actually booted from on the kernel command line (a new
gosd.bootdev-style param, alongside the existing gosd.board/gosd.debug
overrides gosd-init already parses from /proc/cmdline — see
internal/initcfg), and have MountBootPartition prefer/require that device
over walking the candidate list blind. This needs support in every board's
U-Boot config (Rockchip boards' extlinux.conf / boot scripts, Pi boards'
config.txt+start.elf chain), so scope and feasibility per board need
checking before committing to it — may not be uniformly available depending
on what each board's U-Boot/bootloader exposes.

Out of scope for gosd-pcwl (see that bean's "Known residual" note) —
tracked here as a separate, unscoped follow-up.

Cross-reference: [[gosd-pcwl]]
