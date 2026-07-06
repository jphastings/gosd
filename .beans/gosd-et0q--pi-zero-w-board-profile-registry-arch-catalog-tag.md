---
# gosd-et0q
title: 'Pi Zero W board profile: registry, arch, catalog tag'
status: todo
type: task
created_at: 2026-07-06T15:48:45Z
updated_at: 2026-07-06T15:48:45Z
parent: gosd-ajpz
blocked_by:
    - gosd-2j6z
    - gosd-s7fk
    - gosd-06kj
---

Wire it together: internal/boards/pizerow profile (Artifacts: kernel.img + bcm2835-rpi-zero-w.dtb; BootFiles incl. rendered templates + initramfs; FirmwareFiles from the 43430 manifest incl. aliases; no RawWrites; Arch = arm/GOARM=6), registered PUBLIC. Imager catalog device tag: verify the official tag for Pi Zero/Zero W (expect pi1-32bit family) from the v4 os_list like the pi3-64bit fix did — cite evidence. Integration test: fake-artifacts build produces an image whose /app and /init are 32-bit ARM ELFs; no---board now emits three public images. COMPATIBILITY.md column + footnotes in the same PR (per convention), incl. the armv6/performance caveat.
- [ ] Profile + registration + integration tests
- [ ] Catalog tag verified + golden tests
- [ ] COMPATIBILITY.md updated
