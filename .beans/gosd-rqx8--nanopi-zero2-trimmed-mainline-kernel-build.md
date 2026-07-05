---
# gosd-rqx8
title: 'NanoPi Zero2: trimmed mainline kernel build'
status: todo
type: task
priority: low
created_at: 2026-07-05T05:34:03Z
updated_at: 2026-07-05T05:34:03Z
parent: gosd-cwjf
blocked_by:
    - gosd-vcae
---

Mirror build/boards/radxa-zero-3e/kernel/: Dockerized build.sh from a pinned mainline tag, defconfig + committed fragment via merge_config, CONFIG_MODULES=n, everything =y: the same core/initramfs-zstd/vfat/net baseline as the other boards plus RK3528 SoC support, GbE (per research findings), USB gadget stack (per research: dwc2 or dwc3 + configfs functions), GPIO/I2C/SPI (rockchip drivers), serial console. Outputs Image + the board DTB to gitignored out/. Commit the generated full kernel.config with provenance header.
