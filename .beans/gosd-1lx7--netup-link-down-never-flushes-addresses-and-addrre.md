---
# gosd-1lx7
title: 'netup: link-down never flushes addresses and AddrReplace only replaces identical prefixes — replug accumulates stale IPs'
status: todo
type: bug
priority: normal
created_at: 2026-07-31T07:53:06Z
updated_at: 2026-07-31T07:53:06Z
---

Found by review sweep `gosd-fuxs` (gosd-init runtime area), verified
(confidence: likely — netlink semantics, not yet bench-reproduced).

The link-down branch (netup.go:141-154) cancels DHCP and clears the marker
but does no address/route teardown; on the next lease `AddAddr`
(platform_linux.go:37-46) uses `netlink.AddrReplace`, which replaces only
an existing identical address/prefix and otherwise adds alongside.

**Failure scenario:** lease 192.168.1.50/24, unplug past lease expiry,
replug, server hands out .87/24 — eth0 now carries both. Source-address
selection can pick the stale .50 (gateway ARP long gone), so outbound
traffic misroutes or drops; repeated replugs accumulate further. mDNS then
advertises all interface addresses, so hostname.local can resolve to a
dead IP.

**Fix:** add `Links.FlushAddrs(name)` to the seam (AddrDel over
AddrList FAMILY_V4), call it on link-down and before applying a lease
whose address differs; mirror in wifiup on association loss.
