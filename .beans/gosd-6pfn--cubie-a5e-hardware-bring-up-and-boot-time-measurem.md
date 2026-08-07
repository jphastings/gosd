---
# gosd-6pfn
title: 'Cubie A5E: hardware bring-up and boot-time measurement'
status: todo
type: task
priority: deferred
created_at: 2026-08-06T22:34:12Z
updated_at: 2026-08-07T19:11:14Z
parent: gosd-h1wv
blocked_by:
    - gosd-zh95
---

Bench-verify a gosd-built image on JP's Cubie A5E: flash via sdwire, serial console capture, boot to /app, Ethernet (EMAC0) DHCP + mDNS, /data adoption + dataexpand, gosd.toml provisioning, USB gadget if the research bean found it viable, exFAT/disk if applicable. Record a power-on→/app boot-time baseline (later optimization gets its own bean, per fleet convention).

Watch for the known Allwinner-specific risks from the research bean: PMIC regulator dependencies for the SD rail (a kernel missing the AXP drivers may lose the card mid-boot), and BootROM offset mistakes (no SPL banner on serial = wrong offset).

## Todos

- [ ] First boot: SPL/U-Boot banner → extlinux → kernel → gosd-init → /app on serial
- [ ] Ethernet: link, DHCP, mDNS answer
- [ ] Data partition: adoption gate, dataexpand, reboot persistence
- [ ] Provisioning: gosd.toml hand-edit honored
- [ ] Boot-time baseline recorded here + COMPATIBILITY.md footnotes updated with hardware-verified status
- [ ] File follow-up beans for anything found (field-report pattern)

DEFERRED (JP, 2026-08-07): the Cubie A5E is still in the post — bring-up starts when the board physically arrives and goes on the sdwire rig. Software side is fully activated (artifacts v0.9.0, public board, PR #205), so this is hardware-gated only.
