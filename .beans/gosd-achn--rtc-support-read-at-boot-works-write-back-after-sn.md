---
# gosd-achn
title: 'RTC support: read at boot works, write-back after SNTP, per-board verification'
status: todo
type: epic
created_at: 2026-08-07T12:53:05Z
updated_at: 2026-08-07T12:53:05Z
---

JP request (2026-08-07, during cloudflared-ingress planning): make RTCs
actually work on the boards that have one, shrinking the wrong-clock window
(and the ingress TLS-clock window as a side effect).

## Verified state (planning session)

- Fleet kernel.configs already ship CONFIG_RTC_CLASS + RTC_HCTOSYS +
  RTC_SYSTOHC and the right drivers: HYM8563 (nanopi-zero2 — discrete chip at
  i2c 0x51 per its mainline DTS, FriendlyElec 2-pin battery connector),
  RK808-family (rock-4se, radxa-zero-3e PMICs), SUN6I (cubie-a5e SoC), PL031
  (qemu-virt — QEMU seeds it with host time, free CI coverage of the read
  path). No kernel/artifacts change expected.
- The gap is runtime-side: kernel HCTOSYS reads RTC→system at boot, but
  SYSTOHC write-back only fires when the kernel clock is flagged
  NTP-synchronized — timesync's plain settimeofday never sets that flag, so
  the RTC is NEVER WRITTEN and stays wrong forever.
- Pi boards have no RTC at all; timesync's "neither board has a battery-backed
  RTC" comments (timesync.go, guard.go, interfaces.go, platform_linux.go,
  initcfg/config.go) date from the two-board era and are stale.
- Battery caveat: without a coin cell an RTC survives warm reboots (still
  valuable — crash-reboot recovers correct time offline) but not power cuts.

Independent of the ingress epic (gosd-virc).
