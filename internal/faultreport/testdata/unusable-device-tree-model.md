---
error_code: GOSD-DATA-CORRUPT
timestamp: 2026-09-11T11:57:03Z
clock: ntp-synced
uptime: 4m12s
boot: 37
device: QEMU virt (qemu-virt)
image: "myapp 0.1.0 #a1b2c3d4"
---

# myapp crash report

Your myapp device stopped while starting up.

This file was written by the device itself, onto its own SD card, so you can
read it on any computer. Nothing was sent anywhere.

## The problem

The storage this device keeps your data on no longer holds a filesystem it recognises.

## The fix

Plug the card into a computer and salvage what you need from partition 2.

## What to send

If you ask anyone for help, send them **this whole file** rather than a
summary — the section below is the part they need.

## Technical detail

    expanding the data partition: data partition corrupt: /dev/mmcblk0p2 holds nothing (blank space)
