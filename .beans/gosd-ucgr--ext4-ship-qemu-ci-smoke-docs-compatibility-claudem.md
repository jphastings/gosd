---
# gosd-ucgr
title: 'ext4 ship: qemu CI smoke, docs, COMPATIBILITY, CLAUDE.md, minor version'
status: todo
type: task
priority: normal
created_at: 2026-08-07T09:58:20Z
updated_at: 2026-08-07T09:58:28Z
parent: gosd-lfu0
blocked_by:
    - gosd-1c0x
---

Shipping bean for epic gosd-lfu0. A qemu-virt CI job (or an extension of the existing qemu boot tests) formats+grows an ext4 virtio disk through the real disk/ path and reboots to prove adoption; docs and locked decisions catch up; the release notes carry the breaking-default note and the minor version bump.

## Todos

- [ ] qemu-virt smoke: disk.FormatAndMountWith(ext4) on a second virtio disk → write file → fsync pattern → hard kill → reboot → journal replay → file present, volume adopted not reformatted
- [ ] COMPATIBILITY.md: ext4 rows per board (Rockchip fleet + qemu-virt yes; Pi boards no with the kernel-config reason)
- [ ] docs: disk package docs + docs/runtime.md cross-link (journal ≠ data durability; fsync pattern still the story)
- [ ] CLAUDE.md: update the Public API locked decision (disk fs tokens, ext4 default, emmc unchanged) + this epic's outcome
- [ ] Release notes text for the minor bump (breaking default called out); JP tags the release
- [ ] Bench follow-up filed for rock-4se NVMe real-hardware verification (power-cut test rig) if not already covered by a bench bean
