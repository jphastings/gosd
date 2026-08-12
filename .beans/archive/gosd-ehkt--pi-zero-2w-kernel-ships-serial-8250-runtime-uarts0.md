---
# gosd-ehkt
title: pi-zero-2w kernel ships SERIAL_8250_RUNTIME_UARTS=0, the exact trap gosd-md4w fixed on pi-zero-w
status: scrapped
type: bug
priority: normal
created_at: 2026-08-09T19:44:51Z
updated_at: 2026-08-10T08:07:45Z
---

Found while chasing a dead serial console on the bench Pi Zero 2W (2026-08-09). NOT yet proven to be the cause of that silence — see below — but it is a real inconsistency worth closing either way.

The released artifacts/v0.10.0 pi-zero-2w kernel.config carries CONFIG_SERIAL_8250_RUNTIME_UARTS=0 (with NR_UARTS=5, SERIAL_8250_BCM2835AUX=y, SERIAL_8250_CONSOLE=y). That is exactly the state bean gosd-md4w diagnosed on pi-zero-w: bcmrpi_defconfig ships RUNTIME_UARTS=0 and relies on the Pi firmware injecting '8250.nr_uarts=1' on the cmdline; with no injection the 8250 core aborts init, the mini-UART console probe fails -EINVAL and its error path clock-gates the UART, giving a board that boots and works perfectly with ZERO console output.

gosd-md4w's fix — an explicit CONFIG_SERIAL_8250_RUNTIME_UARTS=1 in the fragment, with a WHY comment — was applied ONLY to build/boards/pi-zero-w/kernel.fragment. pi-zero-2w's fragment has no such line (it has 8250/8250_CONSOLE/BCM2835AUX/PL011 but not RUNTIME_UARTS). Curiously the committed pi-3b/kernel.config snapshot shows =1 while pi-zero-w and pi-zero-2w both show =0, so the three Pi boards are not consistent; check what pi-3b's released config actually has before assuming its defconfig differs.

Evidence it may NOT be the whole story: booting the bench Zero 2W with '8250.nr_uarts=1' added to cmdline.txt (the exact parameter the firmware would inject) produced no output; nor did naming both UARTs explicitly ('console=ttyS0,115200 console=ttyAMA0,115200', quiet removed); nor did 'uart_2ndstage=1' in config.txt, which makes the VPU firmware print before Linux exists. Total silence at the firmware stage points at the physical TX->RX path, so that board's wiring must be cleared first (adapter is a healthy CP2102N, single reader, correct baud — see the session's serial notes). Once serial works on that board, re-test whether a stock image (console=serial0, no nr_uarts override) gets a console: if it does NOT, this bean is confirmed and the one-line fragment fix + artifacts release is the answer; if it does, the firmware injects nr_uarts on BCM2837 and this is cosmetic consistency only.

Fix if confirmed: add CONFIG_SERIAL_8250_RUNTIME_UARTS=1 to build/boards/pi-zero-2w/kernel.fragment (mirroring pi-zero-w's comment), audit pi-3b the same way, then the normal tag-first artifacts dance.



## Resolved 2026-08-10: NOT a bug on pi-zero-2w — closing as scrapped

Settled empirically, without a serial console and without macOS being able to read ext4, by booting a purpose-built diagnostic app whose /data was FAT32 (so the host could read its output) and which dumped the kernel's own view of the UART to /data/diag.txt.

1. THE FIRMWARE DOES INJECT IT. /proc/cmdline on the booted board reads:
   'coherent_pool=1M 8250.nr_uarts=1 ... console=ttyS0,115200 quiet init=/init gosd.board=pi-zero-2w panic=10'
   The '8250.nr_uarts=1' is prepended by start.elf (the pinned firmware binary contains both '8250.nr_uarts=1' and '8250.nr_uarts=0' as literals, selected by enable_uart). Note the firmware ALSO rewrites our cmdline.txt's 'console=serial0' to 'console=ttyS0'. So CONFIG_SERIAL_8250_RUNTIME_UARTS=0 in the kernel is overridden at boot on every image we ship, since our config.txt always sets enable_uart=1.

2. THE CONSOLE REGISTERS AND IS ENABLED. From the ring buffer (read via /dev/kmsg, which retains everything despite 'quiet'):
   'Serial: 8250/16550 driver, 1 ports, IRQ sharing enabled'
   '3f215040.serial: ttyS0 at MMIO 0x3f215040 (irq = 71, base_baud = 50000000) is a 16550'
   'printk: legacy console [ttyS0] enabled'
   and /proc/consoles shows 'ttyS0  -W- (EC  p a)  4:64' — enabled and preferred.

3. THE PINS ARE MUXED. GPFSEL1 = 0x00012024 read through /dev/gpiomem: GPIO14 (header pin 8, TXD) and GPIO15 (pin 10, RXD) are both ALT5, the mini-UART function. The kernel's 'bcm2835-aux-uart 3f215040.serial: there is not valid maps for state default' line is a RED HERRING — it means the DT node has no Linux pinctrl default state, which is correct on Pi because the firmware does the muxing when enable_uart=1, and the register read proves it did.

CONSEQUENCE FOR THE BENCH: every software layer is now positively confirmed, so the Zero 2W's dead serial console is definitively a hardware fault — the jumper wire, the adapter's RX input, or the adapter's TXD/RXD silkscreen being labelled from the device's perspective. The Pi is provably driving pin 8. The loopback test (adapter TXD jumpered to its own RXD) is the right next step; JP has confirmed pin 6/pin 8 placement twice.

OPTIONAL, NOT WORTH A RELEASE ON ITS OWN: adding CONFIG_SERIAL_8250_RUNTIME_UARTS=1 to build/boards/pi-zero-2w/kernel.fragment would make the three Pi boards consistent and harden against a firmware change, but it buys nothing today and costs a kernel rebuild plus the artifacts release dance. Fold it in only if a Pi kernel is being rebuilt for another reason. pi-zero-w's explicit =1 (bean gosd-md4w) stays as-is regardless — it was diagnosed on its own hardware and this bean says nothing about BCM2835.
