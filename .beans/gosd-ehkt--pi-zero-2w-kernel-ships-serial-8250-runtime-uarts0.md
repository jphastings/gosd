---
# gosd-ehkt
title: pi-zero-2w kernel ships SERIAL_8250_RUNTIME_UARTS=0, the exact trap gosd-md4w fixed on pi-zero-w
status: todo
type: bug
priority: normal
created_at: 2026-08-09T19:44:51Z
updated_at: 2026-08-09T19:45:16Z
---

Found while chasing a dead serial console on the bench Pi Zero 2W (2026-08-09). NOT yet proven to be the cause of that silence — see below — but it is a real inconsistency worth closing either way.

The released artifacts/v0.10.0 pi-zero-2w kernel.config carries CONFIG_SERIAL_8250_RUNTIME_UARTS=0 (with NR_UARTS=5, SERIAL_8250_BCM2835AUX=y, SERIAL_8250_CONSOLE=y). That is exactly the state bean gosd-md4w diagnosed on pi-zero-w: bcmrpi_defconfig ships RUNTIME_UARTS=0 and relies on the Pi firmware injecting '8250.nr_uarts=1' on the cmdline; with no injection the 8250 core aborts init, the mini-UART console probe fails -EINVAL and its error path clock-gates the UART, giving a board that boots and works perfectly with ZERO console output.

gosd-md4w's fix — an explicit CONFIG_SERIAL_8250_RUNTIME_UARTS=1 in the fragment, with a WHY comment — was applied ONLY to build/boards/pi-zero-w/kernel.fragment. pi-zero-2w's fragment has no such line (it has 8250/8250_CONSOLE/BCM2835AUX/PL011 but not RUNTIME_UARTS). Curiously the committed pi-3b/kernel.config snapshot shows =1 while pi-zero-w and pi-zero-2w both show =0, so the three Pi boards are not consistent; check what pi-3b's released config actually has before assuming its defconfig differs.

Evidence it may NOT be the whole story: booting the bench Zero 2W with '8250.nr_uarts=1' added to cmdline.txt (the exact parameter the firmware would inject) produced no output; nor did naming both UARTs explicitly ('console=ttyS0,115200 console=ttyAMA0,115200', quiet removed); nor did 'uart_2ndstage=1' in config.txt, which makes the VPU firmware print before Linux exists. Total silence at the firmware stage points at the physical TX->RX path, so that board's wiring must be cleared first (adapter is a healthy CP2102N, single reader, correct baud — see the session's serial notes). Once serial works on that board, re-test whether a stock image (console=serial0, no nr_uarts override) gets a console: if it does NOT, this bean is confirmed and the one-line fragment fix + artifacts release is the answer; if it does, the firmware injects nr_uarts on BCM2837 and this is cosmetic consistency only.

Fix if confirmed: add CONFIG_SERIAL_8250_RUNTIME_UARTS=1 to build/boards/pi-zero-2w/kernel.fragment (mirroring pi-zero-w's comment), audit pi-3b the same way, then the normal tag-first artifacts dance.
