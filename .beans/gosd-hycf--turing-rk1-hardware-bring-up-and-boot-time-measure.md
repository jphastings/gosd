---
# gosd-hycf
title: 'Turing RK1: hardware bring-up and boot-time measurement'
status: todo
type: task
created_at: 2026-08-25T10:26:48Z
updated_at: 2026-08-25T10:26:48Z
parent: gosd-bntd
blocked_by:
    - gosd-wf58
---

Real-hardware verification on JP's bench (module + Turing Pi 2 baseboard) once the artifacts release + activation lands: flash via rkdeveloptool or the TP2 BMC, confirm serial console output (UART/baud from the research bean), boot to app, eMMC data-partition durability (four-step fsync pattern), and a power-on-to-app boot-time baseline. Not yet wired up on the bench as of epic creation -- JP is getting it ready in parallel with the software work.
