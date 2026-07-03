---
# gosd-rsrd
title: GPIO/I2C/SPI example app + docs for both boards
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

Prove and document the peripherals story.

- [ ] `examples/blink`: blinks an external LED on a 40-pin header GPIO via github.com/warthog618/go-gpiocdev (character device), pin chosen to exist identically on both board headers (both are RPi-compatible 40-pin; pick physical pin 7 / GPIO4 on Pi, document the RK3566 equivalent line name — find it via `gpioinfo` semantics in the DT and record here)
- [ ] Verify /dev/gpiochipN, /dev/i2c-*, /dev/spidev* appear on both boards with the v0.1 kernels; if a node is missing, fix the kernel config (file bug beans against the kernel tasks)
- [ ] docs/peripherals.md: how to read the pinout for each board, gpiocdev example, i2c (periph.io) example snippet, spi snippet; note which buses need dtoverlay/DT changes and how GoSD exposes that (config.txt template already supports overlays on Pi — document; Radxa DT overlays are a follow-up bean if needed)

## Acceptance
LED blinks on both physical boards from the same example source; docs reviewed against real device node listings captured in this bean.
