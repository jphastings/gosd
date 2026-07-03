---
# gosd-s4t4
title: Acquire bring-up hardware kit (serial adapters, cards, cables)
status: todo
type: task
priority: high
created_at: 2026-07-02T21:17:59Z
updated_at: 2026-07-02T21:17:59Z
parent: gosd-sc9w
blocking:
    - gosd-m9dj
    - gosd-nlzf
---

Physical prerequisites for both hardware bring-up tasks — order alongside the boards. JP owns purchasing; an agent can compile links/prices if asked.

Shopping list:
- [ ] 2× USB-UART adapters, 3.3V logic (FT232RL or CP2102 based), with female jumper wires — one stays wired to each board
- [ ] 2× name-brand microSD cards, 8–32GB, A1/A2 class (SanDisk/Samsung), plus a known-good USB SD reader
- [ ] Power: micro-USB PSU for the Pi (2.5A) and a USB-C supply for the Radxa; NOT laptop ports for bring-up (brown-outs corrupt debugging)
- [ ] 1× micro-USB DATA cable (for Pi USB gadget testing later — the inner "USB" port) and 1× USB-C data cable (Radxa OTG)
- [ ] Ethernet cable to the LAN for the Radxa
- [ ] For v0.3: a few LEDs + 330Ω resistors + breadboard + M-F jumper wires

Wiring reference (record corrections here once verified):
- Pi Zero 2W serial: GND=pin 6, TX=pin 8 (GPIO14) → adapter RX, RX=pin 10 (GPIO15) → adapter TX; 115200n8
- Radxa Zero 3E serial: debug UART on the 40-pin header per https://docs.radxa.com/en/zero/zero3 (confirm pins on arrival); 1500000n8 — note many cheap adapters cannot do 1.5Mbaud reliably; CP2102N and FT232H can, plain CP2102 tops out at 1M (buy accordingly)
- A WPA2 test WiFi network (can be a phone hotspot) with a password we can bake into test images

## Acceptance
Both boards on the bench, serial consoles showing existing-OS boot output (test with the vendor images first — proves wiring before GoSD is in the loop).
