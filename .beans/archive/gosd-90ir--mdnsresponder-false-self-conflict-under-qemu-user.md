---
# gosd-90ir
title: 'mdnsresponder: false self-conflict under qemu user networking'
status: completed
type: bug
priority: normal
created_at: 2026-07-05T15:25:03Z
updated_at: 2026-07-06T15:23:17Z
---

Observed during gosd-27lz's qemu-virt boot validation: after eth0 gets its DHCP lease (10.0.2.15) under qemu's user-mode networking, gosd-init logs 'qemu-hello.local is already being answered by another host at 10.0.2.15' — but 10.0.2.15 IS the device's own address, so the conflict probe is detecting its own announcement (multicast looped back by the slirp network), not another host. Log-only (boot and mDNS answering proceed), but the warning is wrong and would mislead anyone debugging. Likely fix: ignore conflict answers whose address set matches our own interface addresses. Reproduce with scripts/qemu-run.sh on any qemu-virt image.



## Summary of Changes

Root cause: `probeForCollision` in `cmd/gosd-init/internal/mdnsresponder/server.go`
queries for the device's own `<hostname>.local` on the *same* `*mdns.Conn` that
answers those queries (pion/mdns has no API to query without also answering on
the same socket). Under a real LAN the OS never loops multicast back to its own
sender (IncludeLoopback is false, so pion/mdns never enables socket-level
multicast loopback), so the old code's assumption — "any answer we receive
genuinely came from a different host" — held. Under qemu's user-mode (slirp)
networking that assumption breaks: slirp's virtual switch reflects the guest's
own multicast question straight back to the guest regardless of the socket
option (the reflection happens in the host's NAT layer, outside the guest's
network stack), so gosd-init answers its own probe query and then receives
that answer back as if from another host at its own current address.

Fix: `probeForCollision` now takes the querier and an `ownAddrs` accessor
behind small interfaces, and checks the probe answer's address against the
device's *current* interface addresses (fetched fresh at answer time, not
snapshotted at startup, so a DHCP renewal mid-probe cannot cause a stale
comparison) before logging a conflict. A genuinely foreign answer is still
logged unchanged.

Verified two ways:
- Fake-driven unit tests in `collision_test.go`: own-answer is not logged as
  a conflict, a foreign answer still is, the address check uses a freshly
  fetched (not startup-snapshotted) address set, and the existing no-answer
  timeout path is unchanged.
- Reproduced live under qemu-virt with `scripts/qemu-run.sh`: built
  `examples/hello` for `--board=qemu-virt` with the pre-fix code and observed
  the exact false-positive log line from this bean
  (`qemu-hello.local is already being answered by another host at 10.0.2.15`);
  rebuilt with the fix and confirmed the same boot sequence (DHCP lease
  10.0.2.15, mDNS responder restart) produces no conflict log at all, while
  the fake-driven true-conflict test confirms a real second responder is
  still reported.
