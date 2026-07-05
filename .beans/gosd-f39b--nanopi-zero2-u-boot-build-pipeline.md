---
# gosd-f39b
title: 'NanoPi Zero2: U-Boot build pipeline'
status: todo
type: task
priority: low
created_at: 2026-07-05T05:34:03Z
updated_at: 2026-07-05T05:34:03Z
parent: gosd-cwjf
blocked_by:
    - gosd-vcae
---

Mirror build/boards/radxa-zero-3e/uboot/: Dockerized build.sh, pinned mainline U-Boot tag, rkbin blob manifest.json (pinned commit + sha256, no re-hosting), CONFIG_BOOTDELAY=0 fragment, outputs idbloader.img + u-boot.itb to gitignored out/. Follow whatever defconfig the research task identified. Hardware serial verification stays with the bring-up task.
