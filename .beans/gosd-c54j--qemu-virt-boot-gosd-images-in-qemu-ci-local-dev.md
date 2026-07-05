---
# gosd-c54j
title: 'qemu-virt: boot GoSD images in QEMU (CI + local dev)'
status: todo
type: epic
created_at: 2026-07-05T07:07:13Z
updated_at: 2026-07-05T07:07:13Z
parent: gosd-cij4
---

Internal-only third board so the full runtime (gosd-init PID 1, mounts, supervision, DHCP, mDNS, SNTP, gosd.toml, /data) executes on a real kernel in CI and on developer machines BEFORE physical hardware — and as the seed of a future `gosd run` dev loop. Decision 2026-07-05 (JP), recorded in CLAUDE.md.

Shape: qemu-system-aarch64 -M virt boots -kernel Image -initrd initramfs.cpio.zst with the built .img attached as a virtio disk (/dev/vda) — no bootloader emulation. The board is EXCLUDED from default all-board builds and end-user docs.
