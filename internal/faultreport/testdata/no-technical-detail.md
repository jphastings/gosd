---
error_code: NO-SENSOR
timestamp: 2026-09-11T11:57:03Z
clock: ntp-synced
uptime: 4m12s
boot: 37
device: Raspberry Pi Zero 2 W Rev 1.0 (pi-zero-2w)
image: "myapp 0.1.0 #a1b2c3d4"
---

# myapp crash report

Your myapp device stopped while reading the temperature.

This file was written by the device itself, onto its own SD card, so you can
read it on any computer. Nothing was sent anywhere.

## The problem

The configured sensor isn't one this build supports.

## The fix

Write bme280 into config/env/SENSOR on this card.

## What to send

If you ask anyone for help, send them **this whole file** rather than a
summary — the section below is the part they need.

## Technical detail

Nothing was captured for this failure.
