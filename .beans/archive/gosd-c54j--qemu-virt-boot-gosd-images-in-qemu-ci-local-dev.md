---
# gosd-c54j
title: 'qemu-virt: boot GoSD images in QEMU (CI + local dev)'
status: completed
type: epic
priority: normal
created_at: 2026-07-05T07:07:13Z
updated_at: 2026-08-21T01:42:08Z
parent: gosd-cij4
---

Internal-only third board so the full runtime (gosd-init PID 1, mounts, supervision, DHCP, mDNS, SNTP, gosd.toml, /data) executes on a real kernel in CI and on developer machines BEFORE physical hardware — and as the seed of a future `gosd run` dev loop. Decision 2026-07-05 (JP), recorded in CLAUDE.md.

Shape: qemu-system-aarch64 -M virt boots -kernel Image -initrd initramfs.cpio.zst with the built .img attached as a virtio disk (/dev/vda) — no bootloader emulation. The board is EXCLUDED from default all-board builds and end-user docs.

## Summary of Changes

The full runtime — gosd-init as PID 1, mounts, supervision, DHCP, mDNS,
SNTP, the config tree, `/data` — executes on a real kernel in CI and on
developer machines without any hardware. gosd-5wm0 built the virtio arm64
kernel; gosd-2v40 added the board profile and gosd-init's virtio device
support (`/dev/vda1`/`/dev/vda2` alongside the mmcblk candidates); gosd-27lz
added the local runner and the CI boot-to-HTTP smoke test, which builds
`examples/hello` from a REAL artifact download, boots it under
`qemu-system-aarch64 -M virt`, and polls the app's HTTP port until it
answers.

The board is registered with `RegisterInternal`, so it is absent from
`boards.All()`/`IDs()`, from `--board`'s help text, from the default
all-boards build and from catalog generation, while `Find()` still resolves
an explicit `--board=qemu-virt` — the exclusion the epic locked, now matched
by CLAUDE.md's decision and by `internal/repocheck`'s registry-derived
parity tests.

It grew well past a CI fixture: `gosd run` (gosd-wnsj) builds and boots an
image in one command as the developer inner loop, and the qemu jobs have
multiplied into the project's main crash-safety proving ground —
`qemu-expand-data`, `qemu-data-ext4` and `qemu-disk-ext4` each boot a real
image to exercise format, online grow, hard-kill and adoption paths that no
fake could reach.
