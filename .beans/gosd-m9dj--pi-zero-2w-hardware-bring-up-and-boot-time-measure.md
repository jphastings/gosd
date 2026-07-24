---
# gosd-m9dj
title: Pi Zero 2W hardware bring-up and boot-time measurement
status: in-progress
type: task
priority: normal
created_at: 2026-07-02T20:56:21Z
updated_at: 2026-07-24T21:12:39Z
parent: gosd-vmgw
blocked_by:
    - gosd-70b2
    - gosd-eu2x
    - gosd-3zrc
    - gosd-m0vj
    - gosd-fbwa
---

On-hardware validation of the full v0.1 stack on the Pi Zero 2W. Requires a USB-UART adapter on GPIO14/15 (115200n8) and a WiFi network.

Procedure (document actual results in this bean as you go):
- [ ] Build examples/hello image with --wifi-ssid/--wifi-pass, flash to SD (document the dd command used)
- [ ] Serial console shows firmware → kernel → gosd-init logs; capture full boot log as an attachment/snippet in this bean
- [ ] App reachable over WiFi via HTTP within 15s of power-on
- [ ] Measure and record: power-to-kernel, kernel-to-init, init-to-app-running, power-to-HTTP-reachable (use kernel printk timestamps + app log)
- [ ] Pull power mid-run 5 times, confirm it always boots again (FAT is mounted read-only — verify no fsck issues)
- [ ] File a bug bean for every deviation found; list them here

## Acceptance
Boot log captured in this bean, timings recorded, power-to-HTTP under 15s (stretch: under 10s), 5/5 power-cycle survival.

### Bring-up session 1 (2026-07-24) — boots after DTB hand-patch; WiFi blocked on gosd-anyp

- First flash: totally silent, healthy-looking ACT LED. Root cause: **the image
  omits the DTB** (gosd-f59k) — kernel hangs pre-console without it. Bench
  workaround for EVERY pi flash until fixed: copy
  bcm2710-rpi-zero-2-w.dtb from the v0.6.0 artifact cache onto GOSD-BOOT.
- With DTB: full boot to app on serial (115200, adapter-friendly), gosd-init
  healthy, hostname/gosd.toml/wifi-config all load. gosd-pcwl's mount-source
  logging visible in production.
- **WiFi: associate/deauth loop on every AP tried — the full elimination
  matrix, evidence chain, and surviving hypothesis (mainline brcmfmac fwsup
  vs downstream) live in gosd-anyp.** Checklist items DHCP/HTTP/timings/
  power-cycles all blocked behind it. Next experiment queued: boot JP's
  gokrazy card (downstream kernel, same mdlayher library, same silicon).
- Diagnostic technique that carried the session: hand-edits on the FAT boot
  partition (remove 'quiet' from cmdline.txt for verbose kernel; edit
  gosd.toml [wifi]) — no reflash needed for any of it.
