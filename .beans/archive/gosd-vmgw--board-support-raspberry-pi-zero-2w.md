---
# gosd-vmgw
title: 'Board support: Raspberry Pi Zero 2W'
status: completed
type: epic
priority: normal
created_at: 2026-07-02T20:49:54Z
updated_at: 2026-08-21T01:35:34Z
parent: gosd-sc9w
---

Everything needed to boot GoSD on the Pi Zero 2W (BCM2710A1, 4×Cortex-A53, 512MB, arm64).

Boot chain: GPU ROM → bootcode.bin → start.elf (from FAT partition) → loads kernel8.img directly. **No U-Boot.** This is the fast path — keep it that way.

Deliverables: trimmed arm64 kernel (Image → kernel8.img) with builtin drivers (no module loading at all), bcm2710-rpi-zero-2-w.dtb, Raspberry Pi boot firmware files, WiFi firmware blobs, config.txt/cmdline.txt templates, and on-hardware validation.

Serial console for development: GPIO14/15, 115200n8.

## Summary of Changes

Shipped `pi-zero-2w` as a public GoSD board — the fleet's first, and the one
whose shape every later board was measured against. gosd-70b2 built the
trimmed arm64 kernel (`Image` installed as `kernel8.img`, everything builtin,
no module loading); gosd-eu2x assembled the boot side — Raspberry Pi GPU
firmware blobs, `config.txt`/`cmdline.txt` templates and the brcmfmac43436
WiFi firmware manifest; gosd-m9dj validated it on real hardware and recorded
the power-on-to-app baseline. The locked "no U-Boot" fast path held: GPU ROM
to `start.elf` to `kernel8.img`, with no bootloader stage of our own.

The board is registered publicly in `internal/boardset`, has a `kernelspec`
entry, a `build/boards/pi-zero-2w/` tree, fake-artifact fixtures under
`cmd/gosd/testdata/`, and a bring-up row in COMPATIBILITY.md reading
"Complete" — parity that `internal/repocheck`'s board tests now enforce in
both directions.

Peripheral defects found on this board afterwards (e.g. gosd-spjt, the
downstream DTB routing the USB port to `dwc_otg` so no UDC appears) live on
their own beans under the peripherals epic, not here.
