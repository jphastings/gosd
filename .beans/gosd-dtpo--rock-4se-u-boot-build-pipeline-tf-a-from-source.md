---
# gosd-dtpo
title: 'ROCK 4SE: U-Boot build pipeline (TF-A from source)'
status: todo
type: task
priority: normal
created_at: 2026-07-13T12:25:00Z
updated_at: 2026-07-13T13:26:09Z
parent: gosd-cuym
blocked_by:
    - gosd-je2r
---

Mirror `build/boards/radxa-zero-3e/uboot/` (Dockerfile, build.sh, bootdelay0.config, README, .gitignore) with the rkbin-fetch stage **replaced** by a TF-A build stage: clone TF-A at A1's pinned tag → `make PLAT=rk3399 CROSS_COMPILE=aarch64-linux-gnu- M0_CROSS_COMPILE=arm-none-eabi- bl31` (add `gcc-arm-none-eabi` to the apt list) → U-Boot `rock-4se-rk3399_defconfig` + bootdelay0.config merge → `make BL31=<path>/bl31.elf` with **no** ROCKCHIP_TPL (RK3399 DRAM init is open-source in U-Boot TPL). First blob-free Rockchip board; README documents this as the template for future RK3399-class boards.

## Locked decisions

- Outputs `idbloader.img` + `u-boot.itb`; same raw-write offsets as radxa-zero-3e (32768 / 8388608, u-boot end ≤16MiB).
- `manifest.json` gets a `tfa` section (repo, tag, license BSD-3-Clause, note we *compile* it) instead of `rkbin` — check whether `build/artifacts/package.sh` or the workflow provenance step reads the `rkbin` key by name; generalize if so.
- UBOOT_TAG v2026.04 (fleet U-Boot tag) unless A1 found the defconfig missing there.

## Todo

- [ ] Dockerfile with TF-A stage + U-Boot stage (FROM scratch AS artifacts)
- [ ] build.sh reading manifest.json's tfa pins
- [ ] manifest.json (tfa section); generalize package.sh/provenance if rkbin-keyed
- [ ] Real Docker build producing idbloader.img + u-boot.itb; record sizes
- [ ] CI uboot job in build-artifacts.yml
- [ ] README (TF-A-from-source pattern)
