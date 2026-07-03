---
# gosd-uo9f
title: Pure-Go configfs USB gadget library + CDC-ACM serial gadget
status: todo
type: task
priority: normal
created_at: 2026-07-02T21:10:00Z
updated_at: 2026-07-02T21:10:20Z
parent: gosd-jge2
blocked_by:
    - gosd-m9dj
    - gosd-nlzf
---

Headline feature: a GoSD app can present the device as USB hardware. Start with serial (CDC-ACM).

Design first (this API is public, take care): package `gadget` in the gosd module. Declarative: define a Gadget{VendorID, ProductID, Manufacturer, Product, Serial, Functions []Function}, call Apply() → it materializes the configfs tree under /sys/kernel/config/usb_gadget/gosd/, links functions into config c.1, and binds the UDC (first entry in /sys/class/udc). Close() unbinds and tears down. configfs is all file writes — no cgo, no exec.

Plumbing prerequisites: gosd-init mounts configfs at /sys/kernel/config; Pi needs `dtoverlay=dwc2,dr_mode=peripheral` in config.txt — make this a builder flag (`--usb-gadget`) that board profiles translate (Pi: overlay line; Radxa: none needed, the OTG port is the power port and dwc3 handles role)

- [ ] gadget package: core tree builder + ACM function; unit tests against a fake filesystem (io/fs abstraction over the configfs writes)
- [ ] examples/usbserial: echoes lines over /dev/ttyGS0
- [ ] Hardware test both boards: plug the OTG port into a laptop, get /dev/ttyACM0 (macOS: /dev/cu.usbmodem*), record results here — note the Pi Zero 2W data port is the inner micro-USB (USB, not PWR); Radxa is the USB-C OTG/power port

## Acceptance
Both boards enumerate as a serial device on macOS and Linux hosts; echo works; teardown/re-Apply works without reboot.
