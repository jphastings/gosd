---
# gosd-y3vm
title: Bench Pi Zero 2W has no serial console output — hardware fault
status: scrapped
type: bug
priority: normal
created_at: 2026-08-10T10:14:46Z
updated_at: 2026-08-10T12:29:20Z
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



## Kernel build EXONERATED by a stock-OS control (2026-08-10)

JP's hypothesis was that the console problem lay in how we build the kernel. Tested directly and it does not: **stock Raspberry Pi OS Lite is equally silent on this rig.**

Downloaded the official raspios_lite_armhf image, flashed it unmodified except for appending `enable_uart=1` and `uart_2ndstage=1` to its config.txt (its cmdline.txt already ships `console=serial0,115200`), booted, and watched for 3.5 minutes: zero bytes. It shares nothing with GoSD — not our kernel, initramfs, config.txt generation, boot-file selection or gosd-init.

**Proof it actually booted** (no console to watch, so inferred from the card): the image's rootfs partition is ~2GB as flashed; after the boot `diskutil list` showed partition 2 at **15.4GB**, i.e. Raspberry Pi OS ran its first-boot filesystem expansion to fill the 15.9GB card. Only the running OS does that.

Board coverage now, all silent on the same rig: TWO different Pi Zero 2Ws (distinct MACs B8:27:EB:EF:22:24 and B8:27:EB:77:51:55) and one BCM2835 board, under GoSD's 64-bit kernel, GoSD's 32-bit armv6 kernel, and stock Raspberry Pi OS. Two SoCs, two architectures, three kernels, one adapter — the only invariant left is the physical link.

Incidental but worth recording: the third board reports `bcm2708.boardrev=0x9000c1`, which is model 0x0C = **Pi Zero W**, not the Zero 1.3 it was thought to be (a 1.3 is 0x900093). Its `/proc/cmdline` also carries NO firmware-injected `8250.nr_uarts=` — independently reconfirming bean gosd-md4w's finding that BCM2835 firmware doesn't inject it, and therefore that pi-zero-w's explicit CONFIG_SERIAL_8250_RUNTIME_UARTS=1 is load-bearing (ttyS0 did register). That is the exact opposite of the Zero 2W, where the firmware does inject — the split gosd-ehkt documented.

Two leads chased and dismissed: `base_baud = 0` on the PL011 looks alarming but is normal on Pi (it appears in ordinary Pi 4 dmesg output); and a core_freq/baud mismatch would produce GARBAGE, not silence — zero bytes means no edges on the wire at all, which no baud error can cause.

Next step unchanged and now the only step: the loopback test.



## Loopback PASSED (2026-08-10) — adapter cleared, and what that does NOT rule out

With the adapter's TXD jumpered to its own RXD, the probe round-tripped all 20 bytes: `PASS - adapter echoed 20 bytes: b'GOSD-LOOPBACK-PROBE\n'`. So the CP2102N, its macOS driver, the USB path and the host-side capture method are all healthy. Combined with the stock-Raspberry-Pi-OS control above, both ends are now positively proven working and the fault is strictly the link between the Pi's pin 8 and the adapter's RX input.

**Important logical caveat: a loopback CANNOT detect a silkscreen inversion.** Jumpering the pin labelled TXD to the pin labelled RXD closes the loop whether or not those labels are correct — if the labels were swapped, the jumper still joins the real TX to the real RX and still passes. So the hypothesis that the adapter's "RXD" pin is actually its output remains fully alive, and would produce exactly the silence seen: the adapter's output wired to the Pi's output, two drivers fighting, nothing decoded at either end.

Remaining candidates, in the order worth testing:
1. **Adapter silkscreen inversion** — move the wire that currently sits on Pi pin 8 to the OTHER adapter pin. 30 seconds, and the loopback did not exclude it.
2. **A broken or intermittent jumper wire** — invisible, extremely common, and the one component that moves between boards (which is why the adapter working on other boards doesn't clear the wires). Swap the wires outright even if they look fine.
3. **Contact at the Pi header** — solder joint or seating on that particular board; less likely now that three boards have behaved identically, unless the wires were the constant.

Useful follow-up if 1 and 2 both fail: reverse the test direction. The adapter's TX is now proven good, so wire it to the Pi's RX (GPIO15, pin 10) and have an app read /dev/ttyS0 and record what arrives into a FAT32 /data. If the Pi receives, that wire and both pin contacts are good in that direction and suspicion narrows hard onto the Pi's TX pin itself.



# CORRECTION (2026-08-10): this is NOT a hardware fault. Everything above diagnosing it as one is WRONG.

JP's original instinct was right and my conclusion was wrong. With the link re-seated as adapter RXD -> Pi pin 8 and adapter TXD -> Pi pin 10, the wiring is now PROVEN good in both directions — and the console is still silent.

**Proof the link works.** The host sent 430 newline-terminated probes and received all 430 back. Critically it sent `GOSD-PROBE-0001\n` and got back `GOSD-PROBE-0001\r\n` — the CR was ADDED, which is a Linux tty line discipline echoing with ONLCR, not an adapter loopback (that would return bytes verbatim). Independently, the Pi's own app logged 3712 bytes of probes arriving on /dev/ttyS0. So: host -> Pi works (pin 10 wire), and Pi -> host works (pin 8 wire), and the Pi's UART transmits at a correct 115200 (the echo decoded perfectly, so the baud is right).

**And yet the console emits nothing.** With that same known-good wiring:
- `console=serial0,115200`, `quiet` removed: zero bytes.
- `console=ttyS0,115200 ignore_loglevel` plus `uart_2ndstage=1`: zero bytes. The board demonstrably booted (its app wrote diag.txt at uptime 7s), /proc/cmdline confirms the kernel received exactly those arguments, and /proc/consoles reports `ttyS0  -W- (EC  p a)` — enabled, preferred, printk, panic-safe.

**So the finding is: the tty write path reaches the wire, and the console printk path does not, on the same UART, in the same boot.** Kernel printk goes through uart_console_write -> serial8250_console_putchar (a polled path that spins on the LSR via wait_for_xmitr); the echo goes through n_tty -> the driver's normal IRQ/FIFO transmit. The former produces nothing while the latter is perfect.

**Invalidated by this correction:** the stock-Raspberry-Pi-OS control recorded above was run BEFORE the re-seat, so its silence proves nothing and does NOT exonerate our kernel — that experiment must be re-run on the known-good wiring before any conclusion is drawn from it. The same applies to every earlier board-swap null.

**Leading lead for whoever picks this up.** Our kernels report `base_baud = 50000000` for the mini-UART (both the Zero 2W at 0x3f215040 and this BCM2835 board at 0x20215040). A conventional working Pi reports `base_baud = 31250000`. base_baud is core_freq/8, so ours implies a 400MHz core where the documented `enable_uart=1` behaviour is to pin core_freq at 250MHz. That discrepancy does NOT explain the baud (the echo decodes correctly at 115200, so the divisor and the real clock agree), but it is a real difference from a stock Pi and is the first thing worth chasing — particularly whether the polled console path mis-waits on a clock/LSR assumption the IRQ path doesn't make.

Next concrete steps: (1) re-run the stock-Raspberry-Pi-OS control on the good wiring — if RPi OS prints and ours doesn't, it is squarely our kernel config; (2) have an app write directly to /dev/ttyS0 to confirm userspace writes arrive while printk does not; (3) compare our fragment's 8250 options against a stock Pi kernel.config, focusing on the console/polled path rather than port registration.



# RESOLVED — NOT A BUG. The console always worked; the CAPTURE TOOL was broken.

Everything above is wrong, including the earlier correction. There is no serial fault on any board, in GoSD's kernel, or in the wiring.

`tio` 3.9, run backgrounded non-interactively as `sleep 999999 | tio -b 115200 -t -L --log-file X /dev/cu.usbserial-0001 > Y`, prints its banner, reports `Connected`, sets the port to 115200 — and then delivers NOT ONE received byte to stdout or to its --log-file. Every 'silence' measurement in this bean came from that. It is indistinguishable from a dead link without a second reader to compare against.

The tell, missed for a day: every capture done with tio read exactly 162 bytes (its own banner) and every capture done with a hand-written Python reader worked perfectly — the loopback test PASSED, and the probe test round-tripped 430/430. Two instruments disagreed and the broken one was believed.

**Proof the console works.** With a raw-termios Python listener (`os.open(O_RDWR|O_NOCTTY|O_NONBLOCK)`, `tcsetattr` CS8|CREAD|CLOCAL at B115200, poll `os.read`):
- Stock Raspberry Pi OS: 21228 bytes of boot log, including `OF: fdt: Machine model: Raspberry Pi Zero W Rev 1.1` and a `raspberrypi login:` prompt that accepted and rejected typed input.
- **GoSD's own armv6 image: a full clean console** — `[gosd] image identity: fd2b6990b157`, `[gosd] boot partition mounted at /boot from /dev/mmcblk0p1`, `[gosd] data partition filesystem: FAT32`, `[gosd] data partition mounted read-write at /data`, `[gosd] provisioning snapshot saved`, `[gosd] started /app (pid 106)`.

**What this invalidates:** the 'hardware fault' diagnosis; the 'software/kernel fault' correction; the stock-Raspberry-Pi-OS control (its silence was tio, not the OS); the conclusion drawn from two Zero 2W board swaps; and the suspicion that a printk path differed from a tty path (the tty echo arrived only because it was read by Python, while the printk was read by tio).

**What survives:** bean gosd-ehkt's finding is untouched — it rests on /proc/cmdline, /proc/consoles and a /dev/gpiomem register read, never on serial capture. Its conclusion that CONFIG_SERIAL_8250_RUNTIME_UARTS=0 is harmless on pi-zero-2w still stands, and this session independently reconfirmed the BCM2835 half (no firmware-injected nr_uarts on the Zero W).

**Lesson worth keeping** (also recorded in the macos-serial-bringup-gotchas memory): never trust silence from an unproven capture path. Prove the reader first — send a probe and watch it echo, or capture a known-chatty boot — before concluding anything from an absence of bytes. Two separate 'obvious' capture recipes failed silently in this session in two different ways (`stty`+`cat` reverting the port to 9600, and tio swallowing everything).

Closing as scrapped: there is nothing to fix.
