---
# gosd-ajpz
title: 'Board support: Raspberry Pi Zero W (armv6, 32-bit)'
status: completed
type: epic
priority: normal
created_at: 2026-07-06T15:48:45Z
updated_at: 2026-08-21T01:35:46Z
---

Fourth board: the original Pi Zero W — BCM2835, 1x ARM1176JZF-S (armv6, 32-bit ONLY), 512MB, brcmfmac43430 SDIO WiFi, dwc2 USB OTG, same GPU-firmware boot flow as the Zero 2W but loading 32-bit kernel.img (no arm_64bit). Board ID: pi-zero-w.

Architectural consequence (ratified 2026-07-06, CLAUDE.md updated): GoSD is no longer arm64-only — builds are per-board arch, GOARM=6 for this board. The multi-arch task below is the keystone; everything else mirrors the pi-zero-2w work.

Differences from pi-zero-2w to keep straight: WiFi blob family is 43430 (brcmfmac43430-sdio.*), NOT 43436; kernel is 32-bit from bcmrpi_defconfig, installed as kernel.img; DTB bcm2835-rpi-zero-w.dtb; config.txt must NOT set arm_64bit=1; Imager device tag family is the Pi1/Zero 32-bit tag (verify from the official os_list, likely pi1-32bit); single slow core — expect slower boot, WPA2 PBKDF2 takes longer; qemu-virt does not cover armv6 (note, do not fix).

## Summary of Changes

Shipped `pi-zero-w` as a public GoSD board, and with it the architectural
change the epic really bought: GoSD stopped being arm64-only. gosd-2j6z was
the keystone — per-board `GOARCH`/`GOARM`, compiling the app and gosd-init
once per architecture a build needs, `GOARCH=arm GOARM=6` here. gosd-s7fk
built the trimmed 32-bit `kernel.img`; gosd-06kj supplied the firmware
manifest, an `arm_64bit`-free `config.txt` and the brcmfmac43430 WiFi blobs;
gosd-et0q registered the board profile, arch and Imager catalog tag; and
gosd-qltr proved the whole chain on real hardware.

The board is registered publicly in `internal/boardset`, has a `kernelspec`
entry, a `build/boards/pi-zero-w/` tree, fake-artifact fixtures and a
"Complete" bring-up row in COMPATIBILITY.md — parity enforced by
`internal/repocheck`.

Two traps this board taught the project outlived it and are recorded in
CLAUDE.md rather than here: a Pi defconfig's `=m` drivers become `=y` in a
no-modules build (gosd-md4w's missing serial console,
`SERIAL_8250_RUNTIME_UARTS=0`), and the pinned rpi tree ships both
mainline-style and downstream-style DTBs whose bindings differ (gosd-1ey5's
sdhost DMA `dma-ranges` patch). Later board-specific defects such as
gosd-dkqb (SPI disabled in the shipped DTB) are tracked on their own beans.
