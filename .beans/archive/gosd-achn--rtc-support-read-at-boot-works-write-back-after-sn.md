---
# gosd-achn
title: 'RTC support: read at boot works, write-back after SNTP, per-board verification'
status: completed
type: epic
priority: normal
created_at: 2026-08-07T12:53:05Z
updated_at: 2026-08-21T06:50:22Z
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


## Summary of Changes

Closed 2026-08-21 (JP) under the convention recorded in CLAUDE.md's Workflow
section: an epic whose implementation has shipped and is CI-proven closes even
when a hardware bench verification is still outstanding — the delivered work
gets recorded as delivered, and the outstanding verification keeps its own bean
rather than holding an epic hostage.

Shipped, all on `main`: the epic's whole gap was that the kernel's SYSTOHC
write-back only fires when the clock is flagged NTP-synchronized, which
timesync's plain `settimeofday` never did — so the RTC was never written and
stayed wrong forever. `cmd/gosd-init/internal/timesync` now writes the synced
time to `/dev/rtc0` itself after every successful SNTP sync, first sync and
resync alike, treating a board with no RTC as an ordinary absence rather than
an error (bean gosd-lx8g, `rtc.go` + `platform_linux.go`'s `RTC_SET_TIME`
ioctl); and the boot-time clock floor no longer clobbers a valid
RTC-provided time that HCTOSYS already installed (bean gosd-jyq8). No kernel
or artifacts change was needed — the fleet configs already carry
`RTC_CLASS`/`HCTOSYS`/`SYSTOHC` and the per-board drivers. qemu-virt's PL031
gives the read path free CI coverage.

**This closure is not a hardware-verification claim.** Which boards actually
bind `/dev/rtc0`, whether time survives a full power cycle with and without a
coin cell, and the resulting per-board COMPATIBILITY.md RTC rows are bean
gosd-5cxc, now a standalone bench bean with no parent.
