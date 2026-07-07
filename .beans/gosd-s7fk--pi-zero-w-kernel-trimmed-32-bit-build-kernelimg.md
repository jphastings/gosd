---
# gosd-s7fk
title: 'Pi Zero W kernel: trimmed 32-bit build (kernel.img)'
status: in-progress
type: task
priority: normal
created_at: 2026-07-06T15:48:45Z
updated_at: 2026-07-07T06:23:50Z
parent: gosd-ajpz
---

Mirror build/boards/pi-zero-2w/ but 32-bit: raspberrypi/linux at the SAME pinned commit as pi-zero-2w, bcmrpi_defconfig base (armv6 target incl. Zero W), fragment mirroring the pi-zero-2w trim (no modules, no media/sound/DRM, brcmfmac SDIO =y for 43430, MMC_BCM2835, DWC2 gadget + configfs functions, RD_ZSTD, VFAT) built with ARCH=arm CROSS_COMPILE=arm-linux-gnueabihf- (standard for rpi armv6 kernels despite the toolchain triplet; note it in the README). Output zImage → out/kernel.img + bcm2835-rpi-zero-w.dtb. Run the build in Docker (background + poll). Commit generated kernel.config with provenance. Add the CI job + packaging entry to build-artifacts.yml.
- [ ] build.sh + fragment + generated kernel.config committed, build ran green locally
- [x] CI job added
- [ ] Boot-tested on hardware (bench)

## Summary of Changes

Mirrored `build/boards/pi-zero-2w/` into `build/boards/pi-zero-w/` for the 32-bit armv6 Zero W kernel: `build.sh` (ARCH=arm, CROSS_COMPILE=arm-linux-gnueabihf-, crossbuild-essential-armhf, bcmrpi_defconfig, same pinned raspberrypi/linux commit as pi-zero-2w), `kernel.fragment` (identical trim to pi-zero-2w's), `kernel.config` (generated, provenance header), `README.md`, `.gitignore`. Added the `pi-zero-w-kernel` job plus packaging/provenance/release entries to `.github/workflows/build-artifacts.yml`.

Full Docker build ran green locally: zImage + bcm2835-rpi-zero-w.dtb produced, kernel.img is ~15.8 MiB (16,526,416 bytes). Every required config option from kernel.fragment survived `make olddefconfig` unchanged. Two options from the pi-zero-2w verification list (CONFIG_SND, CONFIG_EXT3_FS) don't exist as separate Kconfig symbols on this kernel version at all (same on pi-zero-2w's committed config, not a new casualty). CONFIG_DEBUG_KERNEL=y survives despite the fragment turning it off — also true of pi-zero-2w's committed config, so not a new regression from the 32-bit build.

Deliberately out of scope for this task (build pipeline only): no `manifest.go`/`manifest.json` firmware manifest and no `internal/boards` board profile were added — those integrate with Go code this bean was told not to touch, and belong to sibling beans (gosd-06kj for boot files/firmware manifest; presumably a future board-profile bean). COMPATIBILITY.md was left unchanged for the same reason: pi-zero-w isn't buildable end-to-end yet (no board profile registered), so there's no user-visible status to record there until that lands.

Quality gates all green: go test/vet, gofmt, golangci-lint (native + GOOS=linux), actionlint on the workflow file.

Boot-testing remains the bean's open item — no hardware access from this environment.
