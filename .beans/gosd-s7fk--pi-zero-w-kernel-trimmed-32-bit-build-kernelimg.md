---
# gosd-s7fk
title: 'Pi Zero W kernel: trimmed 32-bit build (kernel.img)'
status: todo
type: task
created_at: 2026-07-06T15:48:45Z
updated_at: 2026-07-06T15:48:45Z
parent: gosd-ajpz
---

Mirror build/boards/pi-zero-2w/ but 32-bit: raspberrypi/linux at the SAME pinned commit as pi-zero-2w, bcmrpi_defconfig base (armv6 target incl. Zero W), fragment mirroring the pi-zero-2w trim (no modules, no media/sound/DRM, brcmfmac SDIO =y for 43430, MMC_BCM2835, DWC2 gadget + configfs functions, RD_ZSTD, VFAT) built with ARCH=arm CROSS_COMPILE=arm-linux-gnueabihf- (standard for rpi armv6 kernels despite the toolchain triplet; note it in the README). Output zImage → out/kernel.img + bcm2835-rpi-zero-w.dtb. Run the build in Docker (background + poll). Commit generated kernel.config with provenance. Add the CI job + packaging entry to build-artifacts.yml.
- [ ] build.sh + fragment + generated kernel.config committed, build ran green locally
- [ ] CI job added
- [ ] Boot-tested on hardware (bench)
