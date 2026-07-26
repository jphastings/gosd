---
# gosd-qltr
title: Pi Zero W hardware bring-up
status: in-progress
type: task
created_at: 2026-07-06T15:48:45Z
updated_at: 2026-07-06T15:48:45Z
parent: gosd-ajpz
blocked_by:
    - gosd-et0q
---

Same checklist as the other boards: serial console (GPIO14/15, 115200 — same header position as Zero 2W), flash, boot log captured here, WiFi join (43430) timing, HTTP + mDNS reachable, 5x power-cycle, boot-time measurement (expect slower than Zero 2W: single armv6 core — record the number, adjust README qualitative claims if needed). Requires a Pi Zero W in the hardware kit (gosd-s4t4 updated).


### Bring-up session 1 (2026-07-25, late) — kernel boots; console broken; WiFi unproven

First-ever pi-zero-w image on hardware (board arrived today; image built from
main @ 446a5bb with the day's DTB + WiFi fixes, flashed via dd, no hand-patch).

- Boot 1: LED flickers then dark, ZERO serial output, no hello.local within
  180s. Looked dead — it wasn't.
- Boot 2 with the two FAT-edit diagnostics (config.txt `uart_2ndstage=1` for
  the GPU firmware's own serial log; cmdline `earlycon=uart8250,mmio32,0x20215040`):
  firmware log clean (board rev 9000c1, loads start.elf/fixup/initramfs/
  bcm2835-rpi-zero-w.dtb/kernel.img), then the armv6 kernel (6.18.37+) boots
  and runs normally to ~2.6s where output stops dead at:
  `bcm2835-aux-uart 20215040.serial: error -EINVAL: unable to register 8250 port`
- Root cause (desk-verified in build/boards/pi-zero-w/kernel.config): filed as
  [[gosd-md4w]] — CONFIG_SERIAL_8250_RUNTIME_UARTS=0 leaves the 8250 core
  with no port slots, so the mini-UART console can't register, and the failed
  probe clock-gates the AUX block, killing earlycon (same peripheral) too.
- WiFi: hello.local never appeared across both boots (minutes). Radio/bus
  configs look present; undiagnosable until the console works. Tracked in
  gosd-md4w as blocked-behind.
- Bench techniques that worked: `uart_2ndstage=1` is the Pi-firmware
  equivalent of verbose boot and costs one config.txt line; earlycon gets
  kernel output with a broken console driver (until said driver kills the
  clock). The dtparam warning `Unknown dtparam 'i2c_arm' - ignored` in the
  firmware log is a separate curiosity for the peripheral items later.

Checklist blocked on [[gosd-md4w]] (fragment fix + local gosd build-kernel +
--artifacts-dir bench validation; artifacts-release dance only after proven).


### Bring-up session 2 (2026-07-26, ~00:45-01:45) — console FIXED and proven; SD I/O is the next wall

- gosd-md4w's fix validated on hardware: the RUNTIME_UARTS=1 kernel (local
  gosd build-kernel, ~20min) registers the mini-UART cleanly —
  `Serial: 8250/16550 driver, 1 ports` → `ttyS0 at MMIO 0x20215040` →
  console live, earlycon hands off. Full [gosd] output visible for the
  first time on this board.
- The working console immediately exposed the next layer: [[gosd-1ey5]] —
  mainline sdhost's DMA path throws `DMA addr 0xffffffff+4 overflow` on the
  first partition-scan read; no /dev/mmcblk0p1 ever appears; gosd-init
  fatal-loops on the boot-partition mount. WiFi still untested (never gets
  that far). Both cmdline workarounds eliminated on-bench (force_pio is not
  a mainline param; initcall-blacklisting the DMA controller makes sdhost
  defer forever).
- Also observed: with the controller wedged, gosd-init's reboot-after-fatal
  never fires (10+ min hang) — noted inside gosd-1ey5 as a secondary issue.
- Bench flow note: one boot was wasted flashing a stale image (build step
  silently skipped) — check `ls -l hello-pi-zero-w.img` freshness before dd.

Checklist now blocked on [[gosd-1ey5]] (SD I/O). Console item is DONE.
