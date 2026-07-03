---
# gosd-v370
title: 'Board support: Radxa Zero 3E'
status: todo
type: epic
created_at: 2026-07-02T20:49:55Z
updated_at: 2026-07-02T20:49:55Z
parent: gosd-sc9w
---

Everything needed to boot GoSD on the Radxa Zero 3E (Rockchip RK3566, 4×Cortex-A55, GbE, no WiFi).

Boot chain: BootROM → idbloader.img (TPL+SPL) at LBA 64 → u-boot.itb at LBA 16384 → U-Boot reads extlinux/extlinux.conf from FAT partition 1 → kernel Image + rk3566-radxa-zero-3e.dtb + initramfs. Mainline U-Boot and mainline Linux both support this board — use mainline for everything; no Radxa vendor BSP.

Deliverables: U-Boot build (bootdelay=0), trimmed mainline kernel, extlinux.conf template, bootloader-embedding in the image writer, on-hardware validation.

Serial console for development: 40-pin header debug UART, 1500000n8 (Rockchip default).
