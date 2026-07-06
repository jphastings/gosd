---
# gosd-qltr
title: Pi Zero W hardware bring-up
status: todo
type: task
created_at: 2026-07-06T15:48:45Z
updated_at: 2026-07-06T15:48:45Z
parent: gosd-ajpz
blocked_by:
    - gosd-et0q
---

Same checklist as the other boards: serial console (GPIO14/15, 115200 — same header position as Zero 2W), flash, boot log captured here, WiFi join (43430) timing, HTTP + mDNS reachable, 5x power-cycle, boot-time measurement (expect slower than Zero 2W: single armv6 core — record the number, adjust README qualitative claims if needed). Requires a Pi Zero W in the hardware kit (gosd-s4t4 updated).
