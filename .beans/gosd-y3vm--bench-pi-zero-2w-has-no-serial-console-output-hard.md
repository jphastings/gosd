---
# gosd-y3vm
title: Bench Pi Zero 2W has no serial console output — hardware fault
status: todo
type: bug
priority: normal
created_at: 2026-08-10T10:14:46Z
updated_at: 2026-08-10T10:15:30Z
---

The bench Pi Zero 2W boots and runs perfectly but emits ZERO bytes on the serial console (2026-08-09/10). This blocked, and then rerouted, a whole evening of ext4 bench work — recording it so the ruled-out ground isn't re-walked.

**Every software layer is positively confirmed working** (bean gosd-ehkt, via a diagnostic app that wrote to a FAT32 /data since the console was unusable):
- `/proc/cmdline` = `... 8250.nr_uarts=1 ... console=ttyS0,115200 ...` — firmware injects nr_uarts and rewrites our `console=serial0` to `ttyS0`.
- Ring buffer: `3f215040.serial: ttyS0 at MMIO 0x3f215040 (irq = 71, base_baud = 50000000) is a 16550` then `legacy console [ttyS0] enabled`; `/proc/consoles` shows ttyS0 enabled and preferred.
- `GPFSEL1 = 0x00012024` via /dev/gpiomem: GPIO14 (pin 8, TXD) and GPIO15 (pin 10, RXD) are both **ALT5**, the mini-UART function. The Pi is provably driving pin 8.
- The `bcm2835-aux-uart: there is not valid maps for state default` line is a red herring (firmware does the muxing, and the register read proves it did).

**Host side also cleared:** adapter is a healthy CP2102N (Silicon Labs, enumerated and active in ioreg); exactly one reader on the port (macOS cu.* devices split bytes between concurrent readers — a classic false lead); port confirmed at 115200 via `stty -f`. JP has twice confirmed GND on header pin 6 and the adapter's RXD on pin 8, verifying pin-1 orientation.

**Also tried, all silent:** `8250.nr_uarts=1` forced on the cmdline; `console=ttyS0,115200 console=ttyAMA0,115200` with `quiet` removed (naming both UARTs explicitly rather than trusting the serial0 alias); and `uart_2ndstage=1` in config.txt, which makes the VPU firmware print BEFORE Linux exists — silence there was the first strong sign this is not software at all.

**Next step: the loopback test.** `dist/loopback-test.sh` in the gosd-ssth worktree (or recreate it: raw termios at 115200, write a probe, read it back) with the adapter's own TXD jumpered to its own RXD and the board-side jumpers off. PASS clears the adapter and its driver, leaving the jumper wire or the Pi's header solder joints. FAIL means the adapter, its USB cable or the driver — and the Pi is innocent.

**Untested hypothesis worth 30 seconds first:** cheap USB-serial boards are inconsistent about whose perspective the silkscreen takes — a pin labelled RXD sometimes means "connect to the device's RXD". Swapping which adapter wire sits on pin 8 costs nothing and would explain everything.

Not urgent: the ext4 work it blocked was completed by other channels (MBR inspection + HTTP over WiFi), and gosd-init has no interactive serial surface, so serial is a diagnostic convenience rather than a product feature. But the next board bring-up will want it back.
