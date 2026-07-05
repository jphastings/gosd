---
# gosd-odp7
title: 'NanoPi Zero2: hardware bring-up and boot-time measurement'
status: todo
type: task
priority: low
created_at: 2026-07-05T05:34:03Z
updated_at: 2026-07-05T05:34:03Z
parent: gosd-cwjf
blocked_by:
    - gosd-wskc
---

Requires purchasing: NanoPi Zero2 (2GB recommended), USB-C 5V/2A PSU, serial adapter capable of the confirmed baud (likely 1500000 — CP2102N/FT232H), and an FPC-30 breakout if GPIO testing is wanted. Same checklist as the other boards: capture full serial boot log here, DHCP lease over GbE, HTTP reachable, boot timings, 5x power-cycle survival, file bug beans for deviations. Also verify the USB-C device port enumerates as a gadget (v0.3 feature readiness).
