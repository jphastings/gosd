---
# gosd-md4w
title: pi-zero-w kernel has no working serial console (8250 RUNTIME_UARTS=0)
status: todo
type: bug
created_at: 2026-07-25T22:39:39Z
updated_at: 2026-07-25T22:39:39Z
---

Found during Pi Zero W first hardware boot (gosd-qltr, 2026-07-25). The armv6 kernel boots (verified via uart_2ndstage firmware log + earlycon) but `bcm2835-aux-uart 20215040.serial: error -EINVAL: unable to register 8250 port` — the final kernel config has CONFIG_SERIAL_8250_NR_UARTS=1 but **CONFIG_SERIAL_8250_RUNTIME_UARTS=0**, so the 8250 core starts with zero registered ports and the mini-UART (serial0/ttyS0, this board's console) cannot claim a slot. Double failure: the console never attaches AND the failed aux-uart probe gates the AUX clock, killing earlycon (`uart8250,mmio32,0x20215040` — the same peripheral) mid-boot at ~2.6s, so even diagnostic output stops there.

Fix direction: add `CONFIG_SERIAL_8250_RUNTIME_UARTS=1` to build/boards/pi-zero-w/kernel.fragment — but first verify against the pinned rpi-6.18.y source that serial8250_register_8250_port fails exactly this way with runtime_uarts=0 (the arm64 pi-zero-2w kernel does not share the gap; its console works). Bench validation path: local `gosd build-kernel` + `--artifacts-dir` image — no artifacts release needed to test; ship the fragment change tag-first per docs/artifacts.md once bench-proven.

Also blocked behind this: pi-zero-w WiFi never joins (hello.local absent after minutes, two boots) — cause invisible without a console. The radio/bus configs look present (BRCMFMAC=y + SDIO, MMC_BCM2835=y sdhost, MMC_BCM2835_MMC=y), so diagnose with real eyes once serial works. Full session log in gosd-qltr.
