---
# gosd-s4t4
title: Acquire bring-up hardware kit (serial adapters, cards, cables)
status: completed
type: task
priority: high
created_at: 2026-07-02T21:17:59Z
updated_at: 2026-08-21T03:20:20Z
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

## Addition (2026-07-06): Raspberry Pi Zero W
- [ ] 1x Raspberry Pi Zero W (the original, armv6) + its own microSD card; PSU/cables shared with the Zero 2W (micro-USB). Serial wiring identical to the Zero 2W.


## Summary of Changes

Completed on evidence rather than on ticked boxes. The checkboxes above were
never ticked — purchasing was JP's and nobody came back to the shopping list —
but this bean's stated acceptance has been met for months:

> Both boards on the bench, serial consoles showing existing-OS boot output
> (test with the vendor images first — proves wiring before GoSD is in the
> loop).

- **Pi Zero 2W.** Bring-up gosd-m9dj is complete: a full serial boot log
  captured over the GPIO14/15 UART, HTTP served over WiFi, 5/5 power-cycle
  survival. COMPATIBILITY.md's bring-up table records it as Complete.
- **Radxa Zero 3E.** gosd-nlzf's session 1 (2026-07-24) had two units on the
  bench with a USB-UART attached, and compared serial output under both GoSD
  and Armbian — literally the vendor-image cross-check this bean's acceptance
  asks for — while root-causing the 1.5Mbaud garble to RK3566 TX drive versus
  CP210x input. The workaround (console=ttyS2,115200n8 in extlinux.conf)
  gives fully readable kernel-onward output. Ethernet DHCP lease and mDNS
  both proven in the same session.
- **Pi Zero W** (the 2026-07-06 addition). Bring-up gosd-qltr is complete and
  COMPATIBILITY.md records it; that board and its card cannot have been
  brought up without being bought.
- Cards, reader and PSUs are implied by every flash-and-boot session since,
  and the bench has grown an sdwire rig on top.

One line item is honestly not claimed: the "For v0.3: LEDs + resistors +
breadboard" purchase has no independent evidence anywhere in the tree. It is
not part of this bean's acceptance, which is about serial consoles, and the
work that needs those parts — a real LED blink per board — is the one
remaining bench todo in gosd-nyad, which now lives under the v0.7 milestone
(gosd-dyoi, epic gosd-q6g6). If the parts turn out not to be on the bench,
that is a shopping trip for that bench session, not a reason to hold the v0.1
milestone open behind a hardware kit that demonstrably arrived.
