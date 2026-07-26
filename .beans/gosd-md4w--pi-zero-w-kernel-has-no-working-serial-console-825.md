---
# gosd-md4w
title: pi-zero-w kernel has no working serial console (8250 RUNTIME_UARTS=0)
status: completed
type: bug
created_at: 2026-07-25T22:39:39Z
updated_at: 2026-07-25T22:53:23Z
---

Found during Pi Zero W first hardware boot (gosd-qltr, 2026-07-25). The armv6 kernel boots (verified via uart_2ndstage firmware log + earlycon) but `bcm2835-aux-uart 20215040.serial: error -EINVAL: unable to register 8250 port` — the final kernel config has CONFIG_SERIAL_8250_NR_UARTS=1 but **CONFIG_SERIAL_8250_RUNTIME_UARTS=0**, so the 8250 core starts with zero registered ports and the mini-UART (serial0/ttyS0, this board's console) cannot claim a slot. Double failure: the console never attaches AND the failed aux-uart probe gates the AUX clock, killing earlycon (`uart8250,mmio32,0x20215040` — the same peripheral) mid-boot at ~2.6s, so even diagnostic output stops there.

Fix direction: add `CONFIG_SERIAL_8250_RUNTIME_UARTS=1` to build/boards/pi-zero-w/kernel.fragment — but first verify against the pinned rpi-6.18.y source that serial8250_register_8250_port fails exactly this way with runtime_uarts=0 (the arm64 pi-zero-2w kernel does not share the gap; its console works). Bench validation path: local `gosd build-kernel` + `--artifacts-dir` image — no artifacts release needed to test; ship the fragment change tag-first per docs/artifacts.md once bench-proven.

Also blocked behind this: pi-zero-w WiFi never joins (hello.local absent after minutes, two boots) — cause invisible without a console. The radio/bus configs look present (BRCMFMAC=y + SDIO, MMC_BCM2835=y sdhost, MMC_BCM2835_MMC=y), so diagnose with real eyes once serial works. Full session log in gosd-qltr.

## Verification

Read against raspberrypi/linux @ 63598c83153e19b1f99067ab6df7409de2c111f8 (the
pinned rpi-6.18.y commit in internal/kernelspec). Theory **confirmed in
conclusion, mechanism refined**: RUNTIME_UARTS=0 does cause exactly this
-EINVAL and the failed probe does clock-gate the AUX block, but not via
"no free slot in serial8250_find_match_or_unused" — in 6.18 that path grows
the port array dynamically. The real chain:

- `drivers/tty/serial/8250/8250_platform.c:37` — `nr_uarts` (runtime) is
  initialised to `CONFIG_SERIAL_8250_RUNTIME_UARTS`; overridable by bootarg
  `8250.nr_uarts` (module_param, line 386).
- `8250_platform.c:303-304` — `serial8250_init()` returns -ENODEV when
  `nr_uarts == 0`, so `serial8250_reg.nr = UART_NR` (line 314) and
  `uart_register_driver(&serial8250_reg)` (line 315) never run: the whole
  ttyS uart_driver stays unregistered with `.nr == 0`.
- `8250_core.c:520-523` — `univ8250_console_init()` likewise bails at
  `nr_uarts == 0`: the ttyS console is never registered either.
- `8250_core.c:714-724` — `serial8250_register_8250_port()` no longer fails
  on a full/empty slot table (it grows via `serial8250_setup_port`); its only
  direct -EINVAL is uartclk==0 (lines 709-710), which the bcm2835aux probe
  already guards with -EPROBE_DEFER (`8250_bcm2835aux.c:164-165`). The
  -EINVAL instead comes from `uart_add_one_port` (`8250_core.c:833`) →
  `serial_core_add_one_port` (`serial_core.c:3074-3075`):
  `if (uport->line >= drv->nr) return -EINVAL;` with `drv->nr == 0`.
- `8250_bcm2835aux.c:168-181` — the probe prints the exact observed
  `"unable to register 8250 port"` via dev_err_probe (line 170), then
  `goto dis_clk` → `clk_disable_unprepare(data->clk)` (line 178), disabling
  the AUX clock enabled at line 143 ("this also enables the HW") — the same
  peripheral earlycon `uart8250,mmio32,0x20215040` writes to, which is why
  earlycon died at ~2.6s right after the error line. **(b) confirmed.**

Why our config has 0: it's the **defconfig baseline, not our fragment** —
`arch/arm/configs/bcmrpi_defconfig:658-659` at the pinned commit sets
`CONFIG_SERIAL_8250_NR_UARTS=1` + `CONFIG_SERIAL_8250_RUNTIME_UARTS=0`
explicitly (upstream Kconfig default is 4, `8250/Kconfig:201-205`), and the
fragment never mentioned it. Raspberry Pi ships 0 because their firmware
injects `8250.nr_uarts=<n>` into the cmdline via /chosen (value depends on
firmware state — e.g. forced to 0 on netboot, raspberrypi/firmware#1575).
The zero-w bench boot demonstrably ran with nr_uarts=0 (this -EINVAL is only
reachable then), while pi-zero-2w — whose `bcm2711_defconfig:739-740` has the
same NR_UARTS=5/RUNTIME_UARTS=0 pair and whose recorded kernel.config also
shows RUNTIME_UARTS=0 — got a full working ttyS0 serial boot log on bench
(gosd-m9dj), i.e. the firmware injection evidently happened there. Baking
`CONFIG_SERIAL_8250_RUNTIME_UARTS=1` into the fragment removes the dependency
on that firmware behaviour. NR_UARTS needs no change: the baseline's 1 slot
is exactly the one mini-UART port we register, and RUNTIME_UARTS=1 is within
Kconfig's `range 0 SERIAL_8250_NR_UARTS`. Worth capturing /proc/cmdline on
both Pi bench targets next session to pin down why the injection differed.

Fix landed: `CONFIG_SERIAL_8250_RUNTIME_UARTS=1` in
build/boards/pi-zero-w/kernel.fragment (kernel.config untouched — it records
the last actual build, per the gosd-z9l4 precedent; no kernelspec/test
changes needed since RequiredY derivation only collects `=y` lines).

- [ ] local `gosd build-kernel --board pi-zero-w` rebuild
- [ ] bench boot with working serial console (via `--artifacts-dir`)
- [ ] artifacts release dance (tag-first, then separate Version-bump PR)

**Closed 2026-07-26**: fix bench-proven (see gosd-qltr sessions 2-4); the remaining artifacts-release step is owned by the batch-release window (gosd-36yy / gosd-7wv9), consistent with gosd-1ey5 and gosd-6nl2.
