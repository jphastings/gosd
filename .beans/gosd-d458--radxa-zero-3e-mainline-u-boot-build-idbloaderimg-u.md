---
# gosd-d458
title: 'Radxa Zero 3E: mainline U-Boot build (idbloader.img + u-boot.itb)'
status: in-progress
type: task
priority: normal
created_at: 2026-07-02T21:02:28Z
updated_at: 2026-07-03T18:23:31Z
parent: gosd-v370
---

Build mainline U-Boot for the Radxa Zero 3E with zero boot delay, plus the pinned build script.

Source (locked): mainline U-Boot, latest release tag (>= v2025.01), `radxa-zero-3-rk3566_defconfig` (covers 3E and 3W; it picks the DT at runtime — verify this claim against the defconfig and note findings in this bean). RK3566 needs two blobs, per U-Boot doc/board/rockchip.rst: the DDR-init TPL (`rkbin` ddr blob, ROCKCHIP_TPL=...) and BL31 (`rkbin` rk3568 bl31 — rk3566 uses the rk3568 blobs). Pin the rkbin repo commit + blob paths + sha256 in `build/boards/radxa-zero-3e/manifest.json`.

Config deltas on top of the defconfig: `CONFIG_BOOTDELAY=0` (keeps abort-with-keypress during dev). Do NOT strip distro-boot/bootstd — we rely on it to find extlinux/extlinux.conf on the first FAT partition of the SD card.

Deliverables in `build/boards/radxa-zero-3e/uboot/`: `build.sh` (Dockerized cross-build like the Pi kernel task), committed config fragment, outputs `idbloader.img` and `u-boot.itb`.

- [x] build.sh producing both artifacts reproducibly
- [x] manifest.json for rkbin blobs with sha256
- [ ] Serial-verified on hardware: U-Boot banner appears, finds extlinux.conf on partition 1 (coordinate with bring-up task)

## Acceptance
Clean-machine build outputs idbloader.img + u-boot.itb; boot-tested to the point of loading extlinux.conf.

## Decision note (2026-07-02)
rkbin blobs (DDR TPL, BL31) follow the no-rehosting policy: pinned upstream URL+sha256 in manifest.json, fetched at build time by CI (for the U-Boot build) — the compiled idbloader.img/u-boot.itb we release necessarily embed them, which the Rockchip rkbin license permits; note the license text location in the manifest.

## Defconfig coverage finding

Verified against pinned U-Boot v2026.04 source: `radxa-zero-3-rk3566_defconfig` DOES cover the 3E via runtime DT selection, confirming the bean's claim.

- `CONFIG_OF_LIST="rockchip/rk3566-radxa-zero-3w rockchip/rk3566-radxa-zero-3e"` in the defconfig builds both DTBs into the FIT (the default FDT is just the 3w one).
- `board/radxa/zero3-rk3566/zero3-rk3566.c` reads SARADC channel 1 (hardware-ID resistor) at runtime: 230-270 -> 3w, 400-450 -> 3e.
- `board_fit_config_name_match()` uses that detection to pick which DTB the SPL FIT loader boots; `rk_board_late_init()` sets `fdtfile` for the rest of U-Boot/extlinux accordingly.

One defconfig, one build, hardware auto-detects at boot. Findings recorded in build/boards/radxa-zero-3e/uboot/README.md.

## Summary of Changes

Implemented the build half of this task:

- `build/boards/radxa-zero-3e/manifest.json`: pins the rkbin repo commit (`ecb4fcbe954edf38b3ae037d5de6d9f5bccf81f4`), the DDR-init TPL and BL31 blob paths, and their real sha256 hashes (computed from actual downloads), plus a note on the rkbin license location/permissions per the no-rehosting policy.
- `build/boards/radxa-zero-3e/uboot/`: `Dockerfile` + `build.sh` do a Dockerized cross-build of mainline U-Boot `v2026.04` (latest stable tag; v2026.07 is still rc-only) using `radxa-zero-3-rk3566_defconfig`, merges in `bootdelay0.config` (CONFIG_BOOTDELAY=0), builds with the pinned BL31/TPL blobs (fetched + sha256-verified inside the Dockerfile), and copies `idbloader.img` + `u-boot.itb` out to `out/` (gitignored via a local `.gitignore`).
- Ran the build end-to-end on this machine (Docker 29.5.3, arm64 macOS): it succeeds and produces `idbloader.img` (202,752 bytes) and `u-boot.itb` (1,313,280 bytes). Had to add `python3-dev` and `libgnutls28-dev` to the Dockerfile's apt packages beyond what the doc implies, to satisfy U-Boot host-tool build dependencies (pylibfdt, mkeficapsule) -- noted as a deviation below.
- `README.md` documents build instructions, the pinned inputs, and the boot-chain summary (idbloader.img -> LBA 64, u-boot.itb -> LBA 16384, extlinux.conf from FAT partition 1) for the reader's context -- actually placing them on the card is the image writer's job (separate bean).

Defconfig coverage claim: verified true, see the '## Defconfig coverage finding' section above.

Not done (honesty): the on-hardware serial verification todo (U-Boot banner, extlinux discovery) is NOT done -- no hardware kit available in this environment. Bean stays in-progress; that todo is left unchecked and should be picked up alongside the Radxa Zero 3E hardware bring-up task.
