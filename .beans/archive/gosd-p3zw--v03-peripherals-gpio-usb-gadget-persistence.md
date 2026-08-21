---
# gosd-p3zw
title: 'v0.3 — Peripherals: GPIO, USB gadget, persistence'
status: completed
type: milestone
priority: normal
created_at: 2026-07-02T20:47:23Z
updated_at: 2026-08-21T03:20:51Z
---

Hardware-application capabilities beyond networking. Definition of done:

- Documented, working GPIO/I2C/SPI story with an example app on both boards (character-device API, not sysfs).
- USB OTG gadget support: a GoSD app can present itself as a USB serial device (CDC-ACM) and as a USB Ethernet device, on both boards.
- A writable data partition survives reboots and reflashes of the app.
- App update over the network (A/B scheme) is scoped — spike written, even if implementation slips to v0.4.


## Summary of Changes

Closed retrospectively. GoSD is on v0.6.5, so this milestone had long since
shipped-or-not; leaving it open mis-recorded both halves. Against its own
definition of done:

- **"Documented, working GPIO/I2C/SPI story with an example app on both
  boards (character-device API, not sysfs)" — SHIPPED,** and wider than
  scoped: all three buses are on by default on every board GoSD supports, not
  just the two this bullet named, with per-board pin tables in the runtime
  docs and three worked examples (examples/i2cscan, examples/spiloopback,
  examples/gpioinfo, the last using go-gpiocdev's chardev API as the epic
  locked). **Not hardware-verified:** the sensor read, the SPI loopback and
  the LED blink are all still open, now under v0.7 (gosd-dyoi, epic
  gosd-q6g6).
- **"USB OTG gadget support: USB serial (CDC-ACM) and USB Ethernet, on both
  boards" — HALF SHIPPED.** The pure-Go configfs gadget library and CDC-ACM
  landed (and USB mass storage besides, which was never asked for). USB
  Ethernet — ECM + RNDIS with a built-in DHCP server, gosd-30jz — was never
  started, and is now v0.7 work.
- **"A writable data partition survives reboots and reflashes of the app" —
  SHIPPED** (gosd-xelb), and hardened well past this bullet since: the
  adoption gate, --data-size=expand, and an opt-in ext4 data filesystem.
- **"App update over the network (A/B scheme) is scoped — spike written, even
  if implementation slips to v0.4" — SHIPPED as written.** The spike
  (gosd-v2w1) produced the merged design doc. Implementation did slip, and
  slipped past v0.4 as well: epic gosd-vxal never had a single one of its
  five implementation beans started, and on 2026-08-21 JP scrapped the whole
  chain. Reflashing is the permanent update path; the design record stays in
  docs/design/ab-updates.md, marked decided-against.

So: three of four bullets delivered and one delivered by half, with hardware
verification of the whole peripherals story and the USB Ethernet gadget
carried forward. Child epic gosd-jge2 is closed here with its own honest
ten-shipped / five-moved split; child epic gosd-vxal was scrapped outright
rather than carried forward, because none of it was delivered and JP has
since decided none of it will be.
