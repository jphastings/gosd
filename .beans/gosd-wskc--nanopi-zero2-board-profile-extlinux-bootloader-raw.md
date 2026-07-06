---
# gosd-wskc
title: 'NanoPi Zero2: board profile (extlinux + bootloader raw-writes)'
status: todo
type: task
priority: low
created_at: 2026-07-05T05:34:03Z
updated_at: 2026-07-06T15:48:45Z
parent: gosd-cwjf
blocked_by:
    - gosd-f39b
    - gosd-rqx8
---

Mirror internal/boards/radxazero3e: register board ID nanopi-zero2; Artifacts() = idbloader.img, u-boot.itb, Image, <board>.dtb; RawWrites at byte offsets 32768 and 8388608 with the u-boot.itb size guard; BootFiles = kernel, dtb, initramfs, extlinux/extlinux.conf (console + baud per research findings, init=/init gosd.board=nanopi-zero2); FirmwareFiles empty. Extend the fake-artifacts integration tests; no---board builds must now emit three images. Also add the board to the artifact pipeline (build-artifacts.yml + manifest) and to CLAUDE.md board IDs (already reserved) + README/docs.

## Scope amendment (2026-07-06, JP: progress as far as possible pre-hardware)
Proceed NOW without waiting for U-Boot v2026.07: implement the full profile + fake-artifact integration tests, but register via RegisterInternal (like qemu-virt) so default builds/catalog exclude it — real artifact fetches would 404 on the U-Boot files until the artifact release includes them. Add a clearly-marked TODO + a checklist item here: flip to public Register + add artifacts-pipeline U-Boot entries when gosd-f39b completes. RawWrites offsets/extlinux content: same pattern as radxa (verify DT/console details from the gosd-vcae findings: rk3528-nanopi-zero2.dtb, UART0 1500000, ttyS0).
- [ ] Flip to public registration once U-Boot artifacts publish (gated on gosd-f39b)
