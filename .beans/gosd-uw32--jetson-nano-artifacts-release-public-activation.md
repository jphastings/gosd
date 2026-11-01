---
# gosd-uw32
title: 'Jetson Nano: artifacts release + public activation'
status: todo
type: task
priority: normal
created_at: 2026-11-01T11:18:51Z
updated_at: 2026-11-01T20:01:23Z
parent: gosd-72je
blocked_by:
    - gosd-9o0m
---

Tag-first, bump-second, per CLAUDE.md, the same two-PR shape as gosd-wf58. PR 1 adds dist/jetson-nano.tar.zst to build-artifacts.yml's release upload list, with an artifacts: change file. After the release is published, PR 2 flips RegisterInternal to Register, bumps artifacts.Version via pin-bump.sh, and adds the fake-artifacts fixtures, the COMPATIBILITY.md row and columns, and end-user docs. Those docs must lead with the QSPI prerequisite: boot JetPack 4.5+ once.


## Decide before flipping to Register (from gosd-0yrk)

gosd now refuses a jetson-nano image whose initramfs is over 14 MiB. Once the board is public, a bare `gosd build` builds every public board. So an app using `--ingress cloudflared` (~27 MiB of initramfs) would fail its WHOLE multi-board build because of the Nano alone. Decide here whether that's acceptable, whether all-boards builds should skip an over-limit Nano with a clear warning, or whether lifting the ceiling (gosd-0yrk's option (b), a boot.scr that moves ramdisk_addr_r) must land first.
