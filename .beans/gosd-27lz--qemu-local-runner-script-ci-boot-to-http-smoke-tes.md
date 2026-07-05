---
# gosd-27lz
title: qemu local runner script + CI boot-to-HTTP smoke test
status: todo
type: task
created_at: 2026-07-05T07:07:13Z
updated_at: 2026-07-05T07:07:13Z
parent: gosd-c54j
blocked_by:
    - gosd-2v40
---

The payoff task.
- [ ] scripts/qemu-run.sh: takes an app .img built with --board=qemu-virt, extracts Image+initramfs from the FAT partition (mtools or go-diskfs helper — no root), launches qemu-system-aarch64 -M virt -cpu cortex-a53 -m 512 with the .img as virtio disk, user networking with hostfwd tcp:8080→:80, serial on stdio. Document in docs/runtime.md (developer section): brew install qemu / apt qemu-system-arm.
- [ ] CI job in ci.yml: build examples/hello --board=qemu-virt (real qemu kernel from the artifact cache/release), boot it headless, poll http://localhost:8080 until the hello response arrives (timeout ~120s TCG), fail with the captured serial log on timeout. This is the first time gosd-init ever runs as PID 1 in CI — expect to file bug beans for what it flushes out; list them here.
- [ ] Note the future `gosd run` command as a follow-up bean once this proves stable

## Acceptance
CI red/green reflects real boot success; a developer can run one script locally and curl their app.
