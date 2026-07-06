---
# gosd-ajpz
title: 'Board support: Raspberry Pi Zero W (armv6, 32-bit)'
status: todo
type: epic
created_at: 2026-07-06T15:48:45Z
updated_at: 2026-07-06T15:48:45Z
---

Fourth board: the original Pi Zero W — BCM2835, 1x ARM1176JZF-S (armv6, 32-bit ONLY), 512MB, brcmfmac43430 SDIO WiFi, dwc2 USB OTG, same GPU-firmware boot flow as the Zero 2W but loading 32-bit kernel.img (no arm_64bit). Board ID: pi-zero-w.

Architectural consequence (ratified 2026-07-06, CLAUDE.md updated): GoSD is no longer arm64-only — builds are per-board arch, GOARM=6 for this board. The multi-arch task below is the keystone; everything else mirrors the pi-zero-2w work.

Differences from pi-zero-2w to keep straight: WiFi blob family is 43430 (brcmfmac43430-sdio.*), NOT 43436; kernel is 32-bit from bcmrpi_defconfig, installed as kernel.img; DTB bcm2835-rpi-zero-w.dtb; config.txt must NOT set arm_64bit=1; Imager device tag family is the Pi1/Zero 32-bit tag (verify from the official os_list, likely pi1-32bit); single slow core — expect slower boot, WPA2 PBKDF2 takes longer; qemu-virt does not cover armv6 (note, do not fix).
