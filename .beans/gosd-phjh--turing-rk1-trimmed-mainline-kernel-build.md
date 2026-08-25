---
# gosd-phjh
title: 'Turing RK1: trimmed mainline kernel build'
status: completed
type: task
priority: normal
created_at: 2026-08-25T10:26:28Z
updated_at: 2026-08-25T12:29:48Z
parent: gosd-bntd
blocked_by:
    - gosd-jvtg
---

kernelspec entry for turing-rk1, joining the existing mainline fleet kernel tag (fleetKernelTag). Config fragment + any DTS patches the research bean found necessary (expect none needed for v1 — no header peripherals in scope). build/boards/turing-rk1/kernel/ mirrors radxazero3e's/rock4se's shape. Requires board.jvtg's RegisterInternal to land first (gosd build-kernel resolves the board via internal/boards before internal/kernelspec, per CLAUDE.md).



## Summary of Changes

Landed together with gosd-jvtg (see that bean) due to internal/repocheck's
mechanical coupling between board registration and kernelspec/build-dir
presence. kernel-fragment.config, kernelassets.go, and the kernelspec.go
entry all in the same commit. Verified for real: `gosd build-kernel --board
turing-rk1` succeeded end-to-end (Image, rk3588-turing-rk1.dtb,
kernel.config all produced; every RequiredY assertion held after
olddefconfig), and the real kernel.config is committed + wired into
kernelConfigSnapshotPath.
