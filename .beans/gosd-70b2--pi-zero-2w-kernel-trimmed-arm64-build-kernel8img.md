---
# gosd-70b2
title: 'Pi Zero 2W kernel: trimmed arm64 build (kernel8.img)'
status: todo
type: task
created_at: 2026-07-02T20:56:21Z
updated_at: 2026-07-02T20:56:21Z
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

- [ ] build.sh producing Image + dtb reproducibly in Docker
- [ ] kernel.config committed; comment header stating source repo + pinned commit
- [ ] Boot-tested on hardware via serial before marking done (coordinate with the bring-up task)

## Acceptance
build.sh on a clean machine outputs kernel8.img + dtb; config has CONFIG_MODULES=n; the bring-up task confirms it boots to gosd-init.
