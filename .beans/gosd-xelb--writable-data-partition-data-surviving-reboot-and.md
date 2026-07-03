---
# gosd-xelb
title: Writable data partition (/data) surviving reboot and reflash-of-app
status: todo
type: task
priority: normal
created_at: 2026-07-02T21:10:00Z
updated_at: 2026-07-02T21:10:20Z
parent: gosd-jge2
blocked_by:
    - gosd-cvzt
---

Apps need persistence (config, sensor logs). Everything else is RAM.

Locked v1: partition 2, FAT32, label GOSD-DATA, sized by builder flag `--data-size` (default 1GiB), created by the image writer (go-diskfs can format FAT32; there is no pure-Go mkfs.ext4 and the kernel cannot format — FAT is the honest v1). gosd-init mounts it rw at /data with flush,sync-friendly options; app gets GOSD_DATA=/data. Document limits plainly: no unix permissions/symlinks, not power-loss-robust — apps should write-rename and fsync.

- [ ] Image writer: second partition support + format
- [ ] gosd-init: mount rw with retry, create /data marker file on first boot
- [ ] `gosd build` REUSE case: when flashing a new image version the data partition is recreated (wiped) — that is acceptable for v0.3 but document it loudly; the A/B update spike owns the preserve-across-updates story
- [ ] Pull-power torture test on hardware: 10 cycles while app writes once per second; record corruption findings here — this data decides whether v0.4 needs littlefs/f2fs

## Acceptance
Example app persists a counter across reboots on both boards; limits documented in the runtime contract page.
