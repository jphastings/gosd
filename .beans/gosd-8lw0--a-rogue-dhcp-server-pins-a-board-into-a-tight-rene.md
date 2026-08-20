---
# gosd-8lw0
title: A rogue DHCP server pins a board into a tight renewal loop with lease-time 0
status: completed
type: bug
priority: normal
created_at: 2026-08-12T04:18:42Z
updated_at: 2026-08-20T05:52:18Z
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

- [x] Bound RenewAfter/RebindAfter/ExpireAfter before anything is scheduled from them (in the pure state machine, not `fromDHCPLease` — see below)
- [x] Test: a hostile ACK never schedules a renewal sooner than the floor, initially or on renewal


## Summary of Changes

Lease timers are now bounded before the maintenance loop schedules anything
from them, and the loop enforces a rate limit of its own.

**Where the clamp went — a deliberate divergence from the "Fix" section
above.** Clamping in `fromDHCPLease` alone would not do: that function is
behind `//go:build linux`, so a clamp there cannot be exercised by the
fake-driven tests this repo requires to pass on macOS, and it leaves the
state machine itself trusting whatever a `DHCPClient` hands it. The bound
therefore lives in the pure layer every implementation feeds — new
`cmd/gosd-init/internal/netup/leasetimes.go` — which `wifiup` inherits for
free, since it calls the same `netup.RunDHCP`.

Two layers, because they answer different questions:

- `saneLeaseTimers` fixes the *lease record*: an absent or zero lease time
  (indistinguishable on the wire, and both currently mean "renew now")
  falls back to `defaultLeaseTime` (1h) with the RFC 2131 §4.4.5 T1/T2
  fallbacks derived from it; every timer is confined to
  `[minLeaseTimer, maxLeaseTimer]` = [60s, 24h]; and T1 <= T2 <= lease-time
  ordering is restored, so a server cannot invert them.
- `waitForLeaseTimer` (was `waitUntil`) fixes the *schedule*: it never
  returns sooner than `minLeaseTimer` from now, whatever target it is
  given. That is the guarantee the loop can state on its own — no lease,
  from any client implementation, and no clock step from timesync, can make
  it hold more than one DHCP conversation per minute per interface.

**The floor is 60s** because RFC 2131 §4.4.5 already floors a *client's own*
DHCPREQUEST retransmissions in RENEWING/REBINDING at 60 seconds: no
conforming server can need to hear from us more often than the RFC's own
minimum. The ceiling is 24h: §3.3 permits lease times up to `0xffffffff`
seconds, which is 136 years and also overflows the T2 fallback arithmetic
into a *negative* offset — i.e. `ObtainedAt.Add(RebindAfter)` permanently in
the past, the very busy-loop this bean is about, reachable from a lease time
that is absurdly large rather than absurdly small. `expire * 7 / 8` in
`fromDHCPLease` was reordered to `expire / 8 * 7` so the overflow does not
arise there either.

Neighbouring failure modes covered, so this is a fix for the class rather
than the reproduction: lease time zero; a tiny lease time; the lease-time
option absent entirely; an absurdly large lease (including the "infinite"
lease and its arithmetic overflow); T1 after T2; and timers that turn
hostile only at renewal (every lease is bounded, not just the one discovery
obtained).

The console reports the first lease on an interface whose timers had to be
corrected, naming what the server offered and what was used instead, then
stays quiet — a rogue server offers the same timers on every renewal, and a
line a minute forever is the failure mode `gosd-yx94` fixed elsewhere.

Tests (`leasetimes_test.go`, `dhcp_test.go`) are fake-driven and pass on
macOS. The renewal-floor assertions read the delay the code asked the fake
clock for, rather than advancing the clock and watching for an effect: the
latter is racy in the direction that matters (asserting something did *not*
happen yet), and a first draft of these tests passed against deliberately
reverted fixes because of it.

Also updated: `dhcp_test.go`'s existing leases used 10s/20s/30s timers,
which are below the new floor; they now use minutes.
