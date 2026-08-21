---
# gosd-q6g6
title: 'Peripherals: bench verification and the USB Ethernet gadget'
status: todo
type: epic
created_at: 2026-08-21T03:19:44Z
updated_at: 2026-08-21T03:19:44Z
parent: gosd-dyoi
---

The unfinished remainder of v0.3's peripherals epic (gosd-jge2), carried into
v0.7. gosd-jge2 itself stays under v0.3 and closed there — ten of its fifteen
children delivered in that release — so only these five moved.

**Three of the five are code-complete and wait only on a bench pass.** Each
has exactly one unchecked todo, and each of those todos says "leave unchecked"
in the bean itself, because hardware verification was deliberately scoped out
when the work was written:

- gosd-85pt (I2C) — a real sensor responds on each board.
- gosd-fnza (SPI) — a MOSI-to-MISO loopback passes on each board. Its bean
  also flags one thing for the bench specifically: the NanoPi Zero2 FPC pin
  numbers were derived by counting along FriendlyElec's schematic rather than
  read off a label, so check continuity before trusting them for wiring.
- gosd-nyad (GPIO) — a real LED blinks on each board.

Everything those three needed off the bench is done and released, including
the two artifact releases their Rockchip DTS patches required
(artifacts/v0.3.0 for I2C, artifacts/v0.4.0 for SPI), each verified against
the published DTBs.

**The other two are genuine, unstarted work:**

- gosd-30jz — USB Ethernet gadget (ECM + RNDIS) with a built-in DHCP server.
  This is the one v0.3 definition-of-done item that never shipped at all:
  CDC-ACM serial landed (gosd-uo9f), USB Ethernet did not.
- gosd-rsrd — the original umbrella "example app + docs for both boards"
  bean, already marked superseded by the three per-bus beans above. It closes
  when they do; it is not separate work.

## The live bench batch

There is an open bench batch — gosd-ftw7 (the 2026-08 runtime hardening
pass), gosd-vv5o (rock-4se NVMe ext4 power-cut rig), gosd-igk0 (cloudflared
ingress end to end) and gosd-5cxc (per-board RTC) — and the three bench passes
here are natural company for it on a single bench session. Nothing is
re-parented into or out of that batch: those beans stay under their own epics,
and this epic tracks its own three.

## Definition of done

All five children completed: three bench passes recorded in their own beans
with COMPATIBILITY.md updated where a verification tier moves, plus the USB
Ethernet gadget delivered.
