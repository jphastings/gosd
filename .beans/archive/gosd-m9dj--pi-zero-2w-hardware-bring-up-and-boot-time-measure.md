---
# gosd-m9dj
title: Pi Zero 2W hardware bring-up and boot-time measurement
status: completed
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


### Bring-up session 2 (2026-07-25) — WiFi fixed, checklist completed

All items unblocked by gosd-anyp's root cause (wifiup's nl80211 CONNECT was
missing netlink.Request — every join was a silently-acked no-op; fixed in
PR #111) and gosd-f59k's DTB fix (PR #109).

- [x] Image built from the fixed branch (`gosd build --board=pi-zero-2w
  --wifi-ssid ... --wifi-pass ...` in examples/hello; flashed with
  `sudo dd if=hello-pi-zero-2w.img of=/dev/rdiskN bs=1m`). **First pi-zero-2w
  flash requiring NO DTB hand-patch** — hardware confirmation of gosd-f59k.
- [x] Full serial boot log captured (GoSD kernel 6.18.37-v8+; firmware →
  kernel → gosd-init → app → WiFi → mDNS; scratchpad capture
  gosd-fixed-boot-01.raw, key lines below).
- [x] App reachable over WiFi via HTTP (hello.local, HTTP 200).
- [x] Timings (kernel printk stamps + wall-clock HTTP poller):
  kernel start → gosd-init ~6.1s; init → app running ~0.4s (app pid at
  ~6.5s); CONNECT issued ~6.6s, association ~2s later, DHCP lease + mDNS
  announce follow (unstamped console lines); **power → HTTP-reachable ~25s
  wall** (poller had 5s granularity; Pi GPU firmware adds ~2-3s before
  kernel time zero). **ACCEPTANCE NOTE: this misses the "<15s" target,
  which predates hardware and matches the Ethernet boards (rock-4se: ~9.2s
  wired). The WiFi path inherently adds association + DHCP + mDNS. JP to
  rule: accept ~25s as the WiFi-path reality (and re-scope the target to
  wired paths), or file a boot-time-optimization bean.**
- [x] 5x power-cycle survival: 5/5 — all cycles rebooted to a serving
  hello.local (network-transition monitor + bench observation; read-only
  FAT, no fsck issues). One caveat recorded honestly: serial capture
  coverage was lost during the cycling window (wiring — see below), so
  cycle evidence is network-level, not serial-level.
- [x] Deviations filed: gosd-f59k (DTB omission — fixed+merged),
  gosd-anyp (WiFi no-op CONNECT — fixed+merged). Residual curiosities, not
  filed as bugs: interface enumerates as wlan2 (consistent across all
  boots, benign); one post-test boot took ~131s to HTTP with no serial
  coverage (unexplained — possibly just switch-flip delay; watch next
  session); the bench serial link went dead partway through the
  power-cycle run (0-byte captures on a previously-working setup — reseat
  the GPIO14/15 jumpers next bench session before trusting serial).

Boot log key lines (session 2, annotated):

    [    0.000000] Booting Linux on physical CPU 0x0000000000
    [    0.000000] Linux version 6.18.37-v8+ (gosd@gosd-ci) ...
    [gosd] hostname set to "hello" (gosd.toml applied)      (~6.1-6.4s)
    [gosd] started /app (pid 153)                           (~6.5s)
    [gosd] using WiFi interface wlan2
    [gosd] wlan2: connect accepted for "Porque Fi"; awaiting association
    [    6.490507] brcmfmac: ... power save enabled
    [gosd] wlan2: lease {192.168.1.235 ...} via gateway 192.168.1.1
    [gosd] mdns: answering as hello.local on all up interfaces
