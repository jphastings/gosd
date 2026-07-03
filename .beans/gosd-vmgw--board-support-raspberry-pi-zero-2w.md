---
# gosd-vmgw
title: 'Board support: Raspberry Pi Zero 2W'
status: todo
type: epic
created_at: 2026-07-02T20:49:54Z
updated_at: 2026-07-02T20:49:54Z
parent: gosd-sc9w
---

Everything needed to boot GoSD on the Pi Zero 2W (BCM2710A1, 4×Cortex-A53, 512MB, arm64).

Boot chain: GPU ROM → bootcode.bin → start.elf (from FAT partition) → loads kernel8.img directly. **No U-Boot.** This is the fast path — keep it that way.

Deliverables: trimmed arm64 kernel (Image → kernel8.img) with builtin drivers (no module loading at all), bcm2710-rpi-zero-2-w.dtb, Raspberry Pi boot firmware files, WiFi firmware blobs, config.txt/cmdline.txt templates, and on-hardware validation.

Serial console for development: GPIO14/15, 115200n8.
