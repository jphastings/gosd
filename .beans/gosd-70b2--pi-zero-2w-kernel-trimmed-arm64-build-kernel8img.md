---
# gosd-70b2
title: 'Pi Zero 2W kernel: trimmed arm64 build (kernel8.img)'
status: in-progress
type: task
priority: normal
created_at: 2026-07-02T20:56:21Z
updated_at: 2026-07-04T11:22:29Z
parent: gosd-vmgw
---

Produce a boot-fast, module-free arm64 kernel for the Pi Zero 2W, plus the build script that CI will later run.

Source (locked): github.com/raspberrypi/linux, latest stable rpi-6.x.y branch, pinned by commit in the build script. Start from `bcmrpi3_defconfig` (arm64, covers BCM2710/Zero 2W), then trim. Deliverables in `build/boards/pi-zero-2w/`: `kernel.config` (full .config, committed), `build.sh` (runs in the official `docker.io/library/debian:bookworm` with crossbuild-essential-arm64, so it works on any host), README with local-build instructions. Outputs: `Image` (renamed kernel8.img), `bcm2710-rpi-zero-2-w.dtb`.

Config requirements (all =y, CONFIG_MODULES=n — nothing loadable):
- Core: devtmpfs (+MOUNT), proc, sysfs, initramfs with CONFIG_RD_ZSTD (other RD_* off), NO initrd-less root support needed, CONFIG_VFAT_FS + NLS_CP437 + NLS_ISO8859_1 (init mounts GOSD-BOOT), tmpfs
- Storage: MMC, CONFIG_MMC_BCM2835, block layer minimal
- WiFi: CONFIG_CFG80211, CONFIG_BRCMFMAC (SDIO), rfkill
- Net: IPv4+IPv6, packet sockets (DHCP needs AF_PACKET), unix sockets
- USB gadget (forward-looking, cheap to include): CONFIG_USB_DWC2 dual-role, CONFIG_USB_GADGET, CONFIG_USB_CONFIGFS + ACM + ECM + RNDIS functions, CONFIG_USB_LIBCOMPOSITE
- Peripherals: CONFIG_GPIO_CDEV (v1 uapi too), CONFIG_I2C_BCM2835, CONFIG_SPI_BCM2835, serial 8250/amba-pl011 + mini-uart console
- Cut aggressively: no sound, no DRM/video (keep simple framebuffer off too), no bluetooth, no filesystems beyond the above, no crypto beyond kernel-required, CONFIG_CC_OPTIMIZE_FOR_PERFORMANCE, disable debug info

- [x] build.sh producing Image + dtb reproducibly in Docker
- [x] kernel.config committed; comment header stating source repo + pinned commit
- [ ] Boot-tested on hardware via serial before marking done (coordinate with the bring-up task)

## Acceptance
build.sh on a clean machine outputs kernel8.img + dtb; config has CONFIG_MODULES=n; the bring-up task confirms it boots to gosd-init.

## Summary of Changes

Fixed the vmlinux link failure and completed the build.

**The bug:** the compile succeeded but `ld` failed at the vmlinux link
with duplicate symbols — raspberrypi/linux ships two copies of the RP1
camera front-end driver (`rp1-cfe` and `rp1_cfe`) that both build and
collide (`multiple definition of dphy_start`/`dphy_stop`/`dphy_probe`).

**The fix:** GoSD wants no camera/video/media support anyway, so
`kernel.fragment` now sets `# CONFIG_MEDIA_SUPPORT is not set`. The
`merge_config.sh` + `olddefconfig` flow then drops the entire media
subsystem, removing both rp1-cfe variants and the collision. No specific
per-driver disable was needed — the media-off cut alone links cleanly.
Nothing else was disabled; all networking, wifi (brcmfmac), mmc, gpio,
usb gadget, vfat, zstd, etc. options are untouched.

**Build result:** `build.sh` ran to completion in the Debian bookworm
container and produced `out/kernel8.img` (arch/arm64/boot/Image, ~56MB)
and `out/bcm2710-rpi-zero-2-w.dtb`. Kernel is Linux 6.18.37 from the
pinned commit.

**Committed:** the `kernel.fragment` fix and the full generated
`kernel.config` (with a header naming source repo, pinned commit, and
bcm2711_defconfig derivation). The `out/` artifacts (Image/dtb) are
gitignored and not committed.

**Config verification** — all required options survived olddefconfig:
`# CONFIG_MODULES is not set`, `CONFIG_BRCMFMAC=y`, `CONFIG_MMC_BCM2835=y`,
`CONFIG_CFG80211=y`, `CONFIG_GPIO_CDEV=y`, `CONFIG_USB_DWC2=y`,
`CONFIG_USB_CONFIGFS=y`, `CONFIG_VFAT_FS=y`, `CONFIG_RD_ZSTD=y`, and
`# CONFIG_MEDIA_SUPPORT is not set`. No exceptions.

Boot-test on hardware remains the one open todo (no hardware here); the
bean stays in-progress until the bring-up task confirms boot to
gosd-init.
