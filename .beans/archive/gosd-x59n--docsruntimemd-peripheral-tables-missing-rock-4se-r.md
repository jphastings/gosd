---
# gosd-x59n
title: docs/runtime.md peripheral tables missing rock-4se rows
status: completed
type: bug
priority: normal
created_at: 2026-07-23T13:35:15Z
updated_at: 2026-07-23T14:49:39Z
---

Noticed during hardware bring-up (gosd-sz6p, 2026-07-23): docs/runtime.md's 'GPIO, I2C, SPI' section documents per-board bus device paths and header pins for pi-zero-2w, pi-zero-w, radxa-zero-3e, and nanopi-zero2 — but rock-4se (activated in gosd-h8a8) appears nowhere in the file. The board has three header I2C buses enabled by its DTS patch (i2c7 pins 3/5 Pi-position, i2c2 pins 27/28, i2c6 pins 29/31 — per build/boards/rock-4se/kernel/patches/0001-enable-header-i2c.patch), plus SPI and GPIO enablement, none documented. Add the rows (I2C table, SPI table, gpiochip numbering) confirmed against the bring-up findings; hardware verification is happening in gosd-sz6p so real /dev/i2c-N numbering can be captured from the boot there.

Hardware verification from gosd-sz6p bring-up (2026-07-23), Qwiic Button at 0x6f:
- rock-4se /dev/i2c-2 = header pins 27 (SDA2) / 28 (SCL2) — device ACK confirmed
- rock-4se /dev/i2c-7 = header pins 3 (SDA7) / 5 (SCL7) — device ACK confirmed; this is the Pi-equivalent position and should be the headline row
- rock-4se /dev/i2c-6 = header pins 29 (SCL6) / 31 (SDA6) — device ACK confirmed; all three buses hardware-verified
Write the runtime.md I2C row(s) with /dev/i2c-7 + pins 3/5 as primary, noting i2c2 and i2c6 as additional enabled header buses (unique among our boards — worth a note column entry).



## Summary of Changes

Added rock-4se rows to docs/runtime.md's 'GPIO, I2C, SPI' section: I2C table (/dev/i2c-7 pins 3/5 as the headline Pi-position row, with i2c-2 pins 27/28 and i2c-6 pins 29/31 called out in the notes column — all three device-ACK-verified against a Qwiic Button during gosd-sz6p bring-up), SPI table (/dev/spidev1.0, pins 19/21/23/24, marked schematic-derived since SPI wasn't exercised on hardware), and the GPIO gpiochip-numbering bullet/table (gpiochip0-gpiochip4, 32 lines each, hardware-confirmed via examples/gpioinfo, with the pin 27 = GPIO2_A0 = gpiochip2 line 0 worked example).

Also updated COMPATIBILITY.md's rock-4se-related footnotes ([^rock4se-otg], [^rock4se-nvme], [^rock4se-emmc]) and the shared [^emmc]/[^usb-gadget]/[^i2c]/[^gpio] footnotes plus the intro blockquote to reflect what gosd-sz6p's bring-up actually hardware-verified on this board (OTG port pinning, NVMe throughput, I2C buses, GPIO enumeration, CDC-ACM gadget, eMMC ErrNoEMMC path) without claiming any other board's rows are now verified.
