---
# gosd-f39b
title: 'NanoPi Zero2: U-Boot build pipeline'
status: todo
type: task
priority: low
created_at: 2026-07-05T05:34:03Z
updated_at: 2026-07-06T02:26:22Z
parent: gosd-cwjf
blocked_by:
    - gosd-vcae
---

Mirror build/boards/radxa-zero-3e/uboot/: Dockerized build.sh, pinned mainline U-Boot tag, rkbin blob manifest.json (pinned commit + sha256, no re-hosting), CONFIG_BOOTDELAY=0 fragment, outputs idbloader.img + u-boot.itb to gitignored out/. Follow whatever defconfig the research task identified. Hardware serial verification stays with the bring-up task.

## Gate note (2026-07-06, from gosd-vcae research)
Mainline U-Boot support (configs/nanopi-zero2-rk3528_defconfig, with USB_GADGET/ROCKUSB enabled) lands in v2026.07, which is NOT yet released (rc only). Per project preference for stable pins, wait for the v2026.07 release tag before building; rkbin blobs needed: rk3528_ddr_1056MHz_v1.13.bin + rk3528_bl31_v1.21.elf (same redistribution license as the RK3566 blobs). Recheck the U-Boot release page early August 2026.
