---
# gosd-bbuh
title: 'qemu-run.sh: pass extra qemu args via QEMU_EXTRA_ARGS'
status: completed
type: task
priority: normal
created_at: 2026-07-17T10:16:13Z
updated_at: 2026-07-17T10:19:49Z
---

internal/qemurun.Options.ExtraArgs exists as the documented escape hatch for qemu flags the wrapper doesn't model, but internal/cmd/qemuboot (and so scripts/qemu-run.sh and every already-built-image workflow) has no way to reach it — only QEMU_DISPLAY is exposed. Downstream need driving this: betamin's qemu test loop attaches a VVFAT media drive and a virtio-sound device to an already-built image.

## Locked decisions

- Env var QEMU_EXTRA_ARGS on qemuboot, newline-separated (one qemu argument per line) so paths containing spaces survive; blank lines ignored. Whitespace-splitting was rejected for exactly the file=fat:ro:/path/with spaces case.
- Parsing lives in internal/qemurun (exported helper) with unit tests; qemuboot stays a thin main.
- scripts/qemu-run.sh's header documents the variable alongside QEMU_DISPLAY.

## Todo

- [x] qemurun.ParseExtraArgsEnv + tests
- [x] qemuboot wires QEMU_EXTRA_ARGS through Options.ExtraArgs
- [x] qemu-run.sh header note

## Summary of Changes

Branch `bean/qemuboot-extra-args` (PR #93): `qemurun.ParseExtraArgsEnv` (newline-separated, blank lines dropped, table-driven test incl. a spaces-in-path case), wired into qemuboot as `QEMU_EXTRA_ARGS`, documented in qemu-run.sh's header next to QEMU_DISPLAY. Options.ExtraArgs itself was already implemented and tested (appended last); this only exposes it to the already-built-image path. First consumer: betamin's qemu test loop (VVFAT media drive + virtio-sound).
