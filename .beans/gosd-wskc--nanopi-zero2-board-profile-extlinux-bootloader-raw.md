---
# gosd-wskc
title: 'NanoPi Zero2: board profile (extlinux + bootloader raw-writes)'
status: todo
type: task
priority: low
created_at: 2026-07-05T05:34:03Z
updated_at: 2026-07-05T05:34:03Z
parent: gosd-cwjf
blocked_by:
    - gosd-f39b
    - gosd-rqx8
---

Mirror internal/boards/radxazero3e: register board ID nanopi-zero2; Artifacts() = idbloader.img, u-boot.itb, Image, <board>.dtb; RawWrites at byte offsets 32768 and 8388608 with the u-boot.itb size guard; BootFiles = kernel, dtb, initramfs, extlinux/extlinux.conf (console + baud per research findings, init=/init gosd.board=nanopi-zero2); FirmwareFiles empty. Extend the fake-artifacts integration tests; no---board builds must now emit three images. Also add the board to the artifact pipeline (build-artifacts.yml + manifest) and to CLAUDE.md board IDs (already reserved) + README/docs.
