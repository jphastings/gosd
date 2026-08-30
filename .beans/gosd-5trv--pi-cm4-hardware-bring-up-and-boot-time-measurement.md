---
# gosd-5trv
title: 'Pi CM4: hardware bring-up and boot-time measurement'
status: todo
type: task
created_at: 2026-08-30T10:25:55Z
updated_at: 2026-08-30T10:25:55Z
parent: gosd-7676
---

## What

Bench: Turing Pi 2 (v2.4), CM4 in node 1, SDWire on its SD card, wired
Ethernet via the baseboard.

Verify the common core (bean pattern from every prior board bring-up):
build → flash (via SDWire) → boot → serial console → network up (DHCP) →
mDNS + HTTP reachable → power-cycle survival of /data (FAT32 default,
then ext4).

USB gadget mode is explicitly OUT of scope this round (epic's "?"
decision — no OTG dock connected). If a dock becomes available later,
characterizing it is a follow-up, not a blocker for closing this bean.
