---
# gosd-vh82
title: 'Turing RK1: default console is ttyS9, real hardware needs ttyS0'
status: completed
type: bug
priority: normal
created_at: 2026-08-30T07:35:34Z
updated_at: 2026-08-30T07:39:45Z
parent: gosd-bntd
blocked_by:
    - gosd-jvtg
---

Hardware bring-up (bean gosd-hycf) found the board profile's baked-in console=ttyS9 (from DT research: rk3588-turing-rk1.dts's stdout-path names the serial9 alias) causes a kernel panic on real hardware: 'Warning: unable to open an initial console.' -> 'Kernel panic - not syncing: Attempted to kill init!'. Confirmed on the actual board that appending console=ttyS0,115200n8 (overriding the default) boots cleanly all the way through gosd-init and the app. Root cause: the generic 8250/8250_dw serial driver does not number this UART by its DT alias index -- this board's DTS has exactly one enabled UART node, so it becomes ttyS0 regardless of the alias being named serial9. Fix: internal/boards/turingrk1/templates/extlinux.conf.tmpl's hardcoded console=ttyS9 -> console=ttyS0 (and every comment/doc/test referencing ttyS9). This is a correction to a locked decision recorded in gosd-jvtg and the epic's Research outcome section (gosd-k4w2) -- update those too, per CLAUDE.md's 'stop and say so' rule for locked decisions that prove wrong in practice.



## Summary of Changes

Fixed internal/boards/turingrk1/templates/extlinux.conf.tmpl:
console=ttyS9 -> console=ttyS0. Verified on real hardware (Turing RK1 in a
Turing Pi 2, node 4): full boot with no kernel-param workarounds --
U-Boot -> kernel -> gosd-init -> the hello example app, which was reachable
over the network at http://hello.local/ via mDNS. Baud (115200) was already
correct; only the device name was wrong. Updated the epic (gosd-bntd) and
research bean (gosd-k4w2) with this correction per the "stop and say so"
rule for locked decisions that prove wrong in practice.
