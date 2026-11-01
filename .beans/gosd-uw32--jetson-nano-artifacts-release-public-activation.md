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

gosd now refuses a jetson-nano image whose initramfs is over 14 MiB. Once the board is public, a bare `gosd build` builds every public board in one loop (cmd/gosd/build.go), and that loop returns on the first board's error. So an over-limit app stops the run at jetson-nano: boards sorting before it are written, and boards after it are never built.

- **cloudflared doesn't change.** A bare `gosd build --ingress cloudflared` already fails today, because `validateIngress` refuses the Pi Zero W (armv6) up front and suggests `--board=`.
- **tailscale-funnel does.** It supports every board, and `examples/hello` with `--ingress tailscale-funnel` is 18.1 MiB of initramfs (measured 2026-11-01), so it would newly fail. So would any large app or `--with-external` build.

The precedent for a board that can't take a feature (`validateIngress`, `validateUsbGadget`) refuses the whole build BEFORE compiling, naming the incapable board and suggesting `--board=`. The Nano's check can only fire after the initramfs exists, so it currently fails mid-loop.

Decide here whether that's acceptable. The other options are: check every board's initramfs before writing any image, skip an over-limit Nano in all-boards builds with a warning, or land the ceiling lift first (gosd-0yrk's option (b), a boot.scr that moves ramdisk_addr_r).
