---
# gosd-d458
title: 'Radxa Zero 3E: mainline U-Boot build (idbloader.img + u-boot.itb)'
status: todo
type: task
priority: normal
created_at: 2026-07-02T21:02:28Z
updated_at: 2026-07-02T21:17:59Z
parent: gosd-v370
---

Build mainline U-Boot for the Radxa Zero 3E with zero boot delay, plus the pinned build script.

Source (locked): mainline U-Boot, latest release tag (>= v2025.01), `radxa-zero-3-rk3566_defconfig` (covers 3E and 3W; it picks the DT at runtime — verify this claim against the defconfig and note findings in this bean). RK3566 needs two blobs, per U-Boot doc/board/rockchip.rst: the DDR-init TPL (`rkbin` ddr blob, ROCKCHIP_TPL=...) and BL31 (`rkbin` rk3568 bl31 — rk3566 uses the rk3568 blobs). Pin the rkbin repo commit + blob paths + sha256 in `build/boards/radxa-zero-3e/manifest.json`.

Config deltas on top of the defconfig: `CONFIG_BOOTDELAY=0` (keeps abort-with-keypress during dev). Do NOT strip distro-boot/bootstd — we rely on it to find extlinux/extlinux.conf on the first FAT partition of the SD card.

Deliverables in `build/boards/radxa-zero-3e/uboot/`: `build.sh` (Dockerized cross-build like the Pi kernel task), committed config fragment, outputs `idbloader.img` and `u-boot.itb`.

- [ ] build.sh producing both artifacts reproducibly
- [ ] manifest.json for rkbin blobs with sha256
- [ ] Serial-verified on hardware: U-Boot banner appears, finds extlinux.conf on partition 1 (coordinate with bring-up task)

## Acceptance
Clean-machine build outputs idbloader.img + u-boot.itb; boot-tested to the point of loading extlinux.conf.

## Decision note (2026-07-02)
rkbin blobs (DDR TPL, BL31) follow the no-rehosting policy: pinned upstream URL+sha256 in manifest.json, fetched at build time by CI (for the U-Boot build) — the compiled idbloader.img/u-boot.itb we release necessarily embed them, which the Rockchip rkbin license permits; note the license text location in the manifest.
