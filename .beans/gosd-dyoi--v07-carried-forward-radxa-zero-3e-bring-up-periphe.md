---
# gosd-dyoi
title: 'v0.7 — Carried forward: Radxa Zero 3E bring-up and peripheral bench passes'
status: todo
type: milestone
priority: normal
created_at: 2026-08-21T03:19:28Z
updated_at: 2026-08-21T03:19:58Z
---

This milestone exists to carry forward work that did not make its original
release. GoSD is on v0.6.5, but v0.1 and v0.3 stayed open on a handful of
unfinished children — which both stopped those milestones closing and, worse,
implied their releases were still in flight. The unfinished work moved here;
the milestones closed with honest summaries of what shipped and what slipped.

Nothing here is new scope. Every child below arrived from somewhere else.

## What came from where

**From v0.1 (gosd-sc9w, now closed)**

- gosd-v370, "Board support: Radxa Zero 3E". The code all shipped — kernel,
  U-Boot, board profile, image-writer wiring, every one of those beans
  completed. The epic is open solely because its hardware bring-up
  (gosd-nlzf) is unfinished: boot-time baseline, the 5x power-cycle survival
  run, and the gadget / GbE / peripheral checks. Session 1 (2026-07-24)
  proved the board boots, takes a DHCP lease and answers on .local; the
  timings need a bench with readable serial, which needs a CH340 cable (this
  board's 1.5M TX garbles on the bench CP2102N).

**From v0.3 (gosd-p3zw, now closed)**

- gosd-q6g6, "Peripherals: bench verification and the USB Ethernet gadget" —
  the five unfinished children of v0.3's gosd-jge2. gosd-jge2 itself stays under v0.3
  and closed there, because ten of its fifteen children delivered in that
  release and moving the epic would have misattributed them.
**Not carried forward: OTA.** An earlier draft of this milestone adopted
gosd-vxal, "App-slot OTA updates (single method, all boards)" — designed in
v0.3 (the spike gosd-v2w1 landed the A/B design doc, which was that
milestone's actual requirement) but with none of its five implementation
beans ever started. JP dropped the whole chain on 2026-08-21 instead:
reflashing is the permanent and only update path. The design record is kept
in docs/design/ab-updates.md, marked decided-against. So OTA is not v0.7's
problem — it is nobody's.

## Definition of done

Every child above completed. This milestone deliberately claims no scope of
its own: if new v0.7 work is wanted, file it and parent it here on purpose
rather than reading ambition into this bean.
