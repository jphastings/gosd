---
# gosd-x59n
title: docs/runtime.md peripheral tables missing rock-4se rows
status: todo
type: bug
priority: normal
created_at: 2026-07-23T13:35:15Z
updated_at: 2026-07-23T13:59:45Z
---

Noticed during hardware bring-up (gosd-sz6p, 2026-07-23): docs/runtime.md's 'GPIO, I2C, SPI' section documents per-board bus device paths and header pins for pi-zero-2w, pi-zero-w, radxa-zero-3e, and nanopi-zero2 — but rock-4se (activated in gosd-h8a8) appears nowhere in the file. The board has three header I2C buses enabled by its DTS patch (i2c7 pins 3/5 Pi-position, i2c2 pins 27/28, i2c6 pins 29/31 — per build/boards/rock-4se/kernel/patches/0001-enable-header-i2c.patch), plus SPI and GPIO enablement, none documented. Add the rows (I2C table, SPI table, gpiochip numbering) confirmed against the bring-up findings; hardware verification is happening in gosd-sz6p so real /dev/i2c-N numbering can be captured from the boot there.

Hardware verification from gosd-sz6p bring-up (2026-07-23), Qwiic Button at 0x6f:
- rock-4se /dev/i2c-2 = header pins 27 (SDA2) / 28 (SCL2) — device ACK confirmed
- rock-4se /dev/i2c-7 = header pins 3 (SDA7) / 5 (SCL7) — device ACK confirmed; this is the Pi-equivalent position and should be the headline row
- rock-4se /dev/i2c-6 = header pins 29 (SCL6) / 31 (SDA6) — device ACK confirmed; all three buses hardware-verified
Write the runtime.md I2C row(s) with /dev/i2c-7 + pins 3/5 as primary, noting i2c2 and i2c6 as additional enabled header buses (unique among our boards — worth a note column entry).
