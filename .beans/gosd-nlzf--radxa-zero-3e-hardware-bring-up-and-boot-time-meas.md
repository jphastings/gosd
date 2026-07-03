---
# gosd-nlzf
title: Radxa Zero 3E hardware bring-up and boot-time measurement
status: todo
type: task
priority: normal
created_at: 2026-07-02T21:02:28Z
updated_at: 2026-07-02T21:10:20Z
parent: gosd-v370
blocked_by:
    - gosd-c7tk
    - gosd-gbsz
    - gosd-3zrc
    - gosd-m0vj
    - gosd-vtce
---

On-hardware validation on the Radxa Zero 3E. Requires USB-UART on the debug pins (1500000n8) and an Ethernet cable to a DHCP network.

- [ ] Build examples/hello image, flash, capture full serial boot log into this bean (TPL/SPL → U-Boot → kernel → gosd-init → app)
- [ ] App reachable over Ethernet via HTTP; record the DHCP lease appearing in logs
- [ ] Measure and record: power-to-U-Boot, U-Boot handoff time, kernel-to-init, power-to-HTTP-reachable
- [ ] If U-Boot adds more than ~2s, note follow-up options in this bean (SPL falcon mode, CONFIG_BOOTDELAY=-2) and file a follow-up bean
- [ ] 5× power-cycle survival test
- [ ] File bug beans for every deviation; list them here

## Acceptance
Boot log + timings recorded here; power-to-HTTP under 15s (stretch 10s); 5/5 power cycles.
