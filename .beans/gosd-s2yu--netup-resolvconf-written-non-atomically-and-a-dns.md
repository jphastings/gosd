---
# gosd-s2yu
title: 'netup: resolv.conf written non-atomically, and a DNS-less renewal ACK wipes working nameservers'
status: todo
type: bug
priority: normal
created_at: 2026-07-31T07:52:39Z
updated_at: 2026-07-31T07:52:39Z
---

Found by review sweep `gosd-fuxs` (gosd-init runtime area), verified.

`WriteResolvConf` (cmd/gosd-init/internal/netup/resolvconf.go:34-44) uses
`os.WriteFile` (O_TRUNC then write) — observably empty mid-write — and
`onLeaseFor` calls it on every lease including renewals with whatever
`ack.DNS()` returned; an ACK omitting option 6 yields an empty list and a
comment-only resolv.conf.

**Failure scenarios:** (a) Go's resolver re-reads resolv.conf ~every 5s;
a lookup during the truncate window finds no nameservers and fails — once
per renewal, forever. (b) Gateways that only include option 6 in the
initial ACK (a real embedded-router behavior on RENEW) get their working
DNS config replaced with an empty file at first renewal: DNS dies while
the network-up marker stays present.

**Fix:** write temp + fsync + rename (atomic on the RAM rootfs), and skip
the write entirely (log it) when the new DNS list is empty rather than
clobbering a valid one.
