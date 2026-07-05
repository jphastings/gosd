---
# gosd-90ir
title: 'mdnsresponder: false self-conflict under qemu user networking'
status: todo
type: bug
created_at: 2026-07-05T15:25:03Z
updated_at: 2026-07-05T15:25:03Z
---

Observed during gosd-27lz's qemu-virt boot validation: after eth0 gets its DHCP lease (10.0.2.15) under qemu's user-mode networking, gosd-init logs 'qemu-hello.local is already being answered by another host at 10.0.2.15' — but 10.0.2.15 IS the device's own address, so the conflict probe is detecting its own announcement (multicast looped back by the slirp network), not another host. Log-only (boot and mDNS answering proceed), but the warning is wrong and would mislead anyone debugging. Likely fix: ignore conflict answers whose address set matches our own interface addresses. Reproduce with scripts/qemu-run.sh on any qemu-virt image.
