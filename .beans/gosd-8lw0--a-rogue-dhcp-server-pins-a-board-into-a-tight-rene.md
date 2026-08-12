---
# gosd-8lw0
title: A rogue DHCP server pins a board into a tight renewal loop with lease-time 0
status: todo
type: bug
created_at: 2026-08-12T04:18:42Z
updated_at: 2026-08-12T04:18:42Z
---

**Severity: Medium.** Sustained CPU, netlink and card-I/O load on a
single-core embedded board, driven entirely from the LAN, contending with
/app indefinitely.

## Verified

`cmd/gosd-init/internal/netup/platform_linux.go:172-174`:

```go
result.ExpireAfter = ack.IPAddressLeaseTime(0)
result.RenewAfter  = ack.IPAddressRenewalTime(result.ExpireAfter / 2)
result.RebindAfter = ack.IPAddressRebindingTime(result.ExpireAfter * 7 / 8)
```

All three are attacker-chosen `uint32` second counts straight off the wire
(DHCP options 51/58/59). `insomniacslk/dhcp`'s `Duration.FromBytes` does a
bare `Read32()` with no minimum.

`maintainLease` (`netup/dhcp.go:63-94`) schedules the next renewal with
`waitUntil(ctx, deps.Clock, lease.ObtainedAt.Add(lease.RenewAfter))`. With
`RenewAfter == 0` the target is already in the past, so it fires immediately
and calls `deps.DHCP.Renew` at once.

**There is no floor clamp on this path** — which is notable, because the
Discover-phase backoff was deliberately hardened against exactly this, at
`netup/backoff.go:51-55`: *"Never propose a zero-length wait: that would
busy-loop against the DHCP server."* The lesson was learned there and then
not applied to the renewal timers.

## Attack

A rogue DHCP server on the LAN ACKs every request with
`IPAddressLeaseTime=0` (or an explicit `RenewTimeValue=0`). The device loops:
`Renew` -> `onLeaseFor` (netlink `AddrReplace`/`AddrDel`/`RouteReplace`, plus
a write+rename of `/etc/resolv.conf` and marker-file I/O) -> repeat, bounded
only by round-trip latency on the same L2 segment. `wifiup` inherits it — it
calls the same `netup.RunDHCP`.

## Fix

Clamp in `fromDHCPLease`, mirroring the discipline already applied to the
Discover backoff: floor RenewAfter/RebindAfter/ExpireAfter at a sane minimum
(60s is ample for any legitimate server).

## Todos

- [ ] Clamp RenewAfter/RebindAfter/ExpireAfter to a minimum in `fromDHCPLease`
- [ ] Test: an ACK with lease time 0 does not produce a zero-length wait
