---
# gosd-bib8
title: 'Turing RK1: mainline U-Boot build (rkbin DDR-init + BL31 blobs)'
status: completed
type: task
priority: normal
created_at: 2026-08-25T10:26:29Z
updated_at: 2026-08-25T12:29:48Z
parent: gosd-bntd
blocked_by:
    - gosd-jvtg
---

build/boards/turing-rk1/uboot/: Dockerfile + build.sh + manifest.json, mirroring radxa-zero-3e's or nanopi-zero2's rkbin-blob pattern (this board needs rkbin's DDR-init TPL binary + BL31 ELF for RK3588 -- there is no open-source DRAM init for this SoC in mainline U-Boot, unlike RK3399/A527). Blob URLs + sha256 from the research bean. Produces idbloader.img + u-boot.itb at the offsets the research bean confirmed. Requires board.jvtg's board-profile shape to exist so the raw-write offsets/artifact names line up.



## Summary of Changes

Landed together with gosd-jvtg (see that bean). build/boards/turing-rk1/uboot/
{Dockerfile,build.sh,bootdelay0.config,README.md} and manifest.json (rkbin
DDR-TPL + BL31 blob pins, same commit already pinned for radxa-zero-3e/
nanopi-zero2) all in the same commit. Verified for real: build.sh produces
idbloader.img (202752 bytes) and u-boot.itb (1358336 bytes), both fitting
the existing fleet offsets (LBA64/LBA16384) with room to spare before the
16MiB boot-partition start -- confirmed the binman-composed
u-boot-rockchip.bin bean gosd-k4w2 flagged as a possible different shape is
just these same two pieces concatenated; no RawWrites design change
needed.
