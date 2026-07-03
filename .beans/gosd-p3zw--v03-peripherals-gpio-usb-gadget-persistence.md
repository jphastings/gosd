---
# gosd-p3zw
title: 'v0.3 — Peripherals: GPIO, USB gadget, persistence'
status: todo
type: milestone
created_at: 2026-07-02T20:47:23Z
updated_at: 2026-07-02T20:47:23Z
---

Hardware-application capabilities beyond networking. Definition of done:

- Documented, working GPIO/I2C/SPI story with an example app on both boards (character-device API, not sysfs).
- USB OTG gadget support: a GoSD app can present itself as a USB serial device (CDC-ACM) and as a USB Ethernet device, on both boards.
- A writable data partition survives reboots and reflashes of the app.
- App update over the network (A/B scheme) is scoped — spike written, even if implementation slips to v0.4.
