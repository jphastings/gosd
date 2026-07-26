---
# gosd-tdf4
title: 'COMPATIBILITY.md refresh: fold in the week''s hardware bring-ups'
status: completed
type: task
priority: normal
created_at: 2026-07-26T09:43:59Z
updated_at: 2026-07-26T09:51:45Z
---

COMPATIBILITY.md's preamble still claims only the ROCK 4SE has been through hardware bring-up. Audit the whole file against the bring-up beans and bring it up to date:

- Completed bring-ups: rock-4se (gosd-sz6p), nanopi-zero2 (gosd-odp7, incl. eMMC format/serve validation), pi-zero-2w (gosd-m9dj + gosd-anyp netlink.Request WiFi root cause), pi-zero-w (gosd-qltr + kernel fixes gosd-md4w/gosd-1ey5/gosd-6nl2).
- In progress: radxa-zero-3e (gosd-nlzf: boots, DHCP/mDNS proven, serial baud workaround; gosd-zp9s open).
- Keep the hardware-verified-locally vs in-released-artifacts distinction for the Pi kernel fixes (md4w/1ey5/6nl2/spjt fragments land at the next artifacts release).
- GOSD-DATA hardware-exercised on both Pi Zeros (gosd-4ajn/gosd-spjt bench); Zero W gadget reached UDC + host enumeration (gosd-spjt), full mass-storage pass pending zoo-evicted kernels.
- Ethernet hardware-verified on all three wired boards.
- No pi-3b column (internal until gosd-7wv9); preamble may mention epic gosd-xhc3 as in-flight.

## Todos

- [x] Rewrite preamble to the new bring-up reality
- [x] Update stale footnotes (custom-kernel, with-external, pi-dtb, usb-gadget, pi-dwc2, emmc/no-emmc/rock4se-emmc, data-opt-in, i2c/gpio/spi wrinkles, armv6-perf)
- [x] Add footnotes for newly hardware-verified rows (Ethernet, pi-zero-2w WiFi)
- [x] Quality gates
- [x] PR

## Summary of Changes

Docs-only: COMPATIBILITY.md audited top to bottom against the bring-up beans.

- Preamble rewritten: four of five boards through hardware bring-up (rock-4se gosd-sz6p, nanopi-zero2 gosd-odp7, pi-zero-2w gosd-m9dj/gosd-anyp, pi-zero-w gosd-qltr); radxa-zero-3e in progress (gosd-nlzf). Defines the hardware-proven common core per completed board, keeps footnotes as the per-row source of truth outside it, states the local-kernel-vs-released-artifacts distinction once, and mentions the in-flight Pi 3 epic (gosd-xhc3).
- New footnotes: [^eth-verified] (Ethernet hardware-verified on all three wired boards; Zero 3E HTTP check still open) and [^pi-zero-2w-wifi] (netlink.Request root cause, no artifact release needed; gosd-6nl2 hwsim eviction rides the next one).
- Updated stale footnotes: custom-kernel (locally built kernels have now run on hardware via the Zero W fixes), with-external phrasing, pi-dtb (no-hand-patch flash hardware-confirmed), rock4se-emmc + emmc (NanoPi format/mount/serve hardware-verified; ErrNoEMMC branches exercised on rock-4se and radxa-zero-3e), no-emmc + data-opt-in (GOSD-DATA exercised on both Pi Zeros via gosd-4ajn/gosd-spjt), usb-gadget (per-board hardware status, dropped the stale blocked-on-kit text), pi-dwc2 (Zero W reached UDC + host enumeration as Gadget Zero; app gadget still pending the zoo-evicted artifacts release), nanopi-usb (no-UDC bench-confirmed), i2c/spi (release wrinkle resolved — published artifacts ship the patched DTBs), gpio/armv6-perf phrasing.
