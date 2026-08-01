---
# gosd-o6tp
title: 'mdnsresponder: NewServer leaks both multicast sockets on its expected-at-boot failure path — fd exhaustion in PID 1'
status: completed
type: bug
priority: normal
created_at: 2026-07-31T07:53:30Z
updated_at: 2026-08-01T18:00:28Z
---

Found by review sweep `gosd-fuxs` (gosd-init runtime area), verified.

`NewServer` (cmd/gosd-init/internal/mdnsresponder/server.go:39-57) opens
pc4/pc6 and returns on `mdns.Server` error without closing either. pion
mdns v2.1.0's `Server` also doesn't close conns it was handed on its early
returns (and leaks two unicast sockets of its own — upstream issue; per
the no-third-party-PRs rule, record the patch in this bean if we want it
fixed upstream). `Run` retries `NewServer` on every `Changed`
notification, and its own comment says failure is expected at boot before
any interface is up.

**Failure scenario:** ~4 fds burned per failed attempt. A WiFi-only board
flapping association (no lease), or booting networkless and notifying
repeatedly, walks PID 1 toward the default 1024-fd rlimit — after which
DHCP sockets, netlink conns, and device nodes all fail to open and the
network never recovers.

**Fix:** close pc4/pc6 on the error return in NewServer (both nil-guarded).
Optionally rate-limit NewServer attempts. The pion-internal unicast leak:
document/patch here, do not PR upstream without JP.

## Summary of Changes

- `NewServer` (server.go) now closes whichever of pc4/pc6 it opened
  (nil-guarded) before returning `mdns.Server`'s error, closing the leak
  this bean tracks. Introduced a `startResponder` package var (== real
  `mdns.Server` in production) as the seam over the third-party call, so a
  test can force that failure deterministically — independent of whether
  this environment can actually join a multicast group — and observe the
  close: a second, test-driven `Close()` on the same real socket returns a
  wrapped `net.ErrClosed`, which it would not if `NewServer` had left it
  open. `TestNewServerAnswersQueries` (unmodified) continues to cover the
  success path.

- Rate-limit decision: **warranted, implemented.** Traced every call site
  that notifies `Deps.Changed` (netup's `handleLinkEvent`/`onLeaseFor`,
  wifiup's `watchAssociation`/`onLeaseFor`). Most are already paced by
  something real — a DHCP round trip, or wifiup's 3s `associationPollPeriod`
  poll before a loss is even reported — but netup's `!ev.Up && running`
  case calls `ClearNetworkUp` (and so notifies `Changed`) synchronously off
  the raw netlink link-down event, with no software pacing at all: a
  flapping wired link (bad cable, flaky driver) can fire it back-to-back.
  Since the upstream pion leak below survives this bean's own fix, an
  unbounded retry rate under that flap pattern would still drain fds, just
  at half the previous rate. Added `minRestartInterval = 250ms` as a floor
  between `deps.NewServer` retries in `Run`'s `Changed` loop
  (mdnsresponder.go) — short enough to be imperceptible for a genuine
  address change, long enough to cap worst-case retry frequency to ~4/sec.
  Self-contained in this package (no new `Deps` field), so no wiring change
  was needed outside `mdnsresponder`.

## Upstream pion/mdns v2.1.0 leak — for JP, do not send upstream

Read `github.com/pion/mdns/v2@v2.1.0/conn.go` directly (module cache) to
confirm the bean's claim. `Server()` opens its own `unicastPktConnV4`/
`unicastPktConnV6` at lines 131-165 (guarded individually — a bind failure
on either just logs a warning and leaves that one nil), then has five
later error returns that never close them:

- line 255 `return nil, errNoUsableInterfaces` — **this is gosd-init's
  everyday expected-at-boot case**: pc4/pc6 open fine, but
  `net.Interfaces()` has nothing `FlagUp` yet, so this upstream leak fires
  on essentially every boot-time retry regardless of this bean's own fix.
- line 258 `return nil, errNoPositiveMTUFound`
- line 264 `return nil, errJoiningMulticastGroup`
- line 269 `return nil, err` (`dstAddr4` resolve — effectively unreachable,
  the address is a package constant)
- line 274 `return nil, err` (`dstAddr6` resolve — same)

Suggested patch (mirrors this bean's own fix — nil-guarded close of both,
called at each of the five leaky returns above):

```diff
--- a/conn.go
+++ b/conn.go
@@ -252,15 +252,25 @@ func Server(
 	}
 
 	if len(ifacesToUse) == 0 {
+		closeUnicastConns(unicastPktConnV4, unicastPktConnV6)
 		return nil, errNoUsableInterfaces
 	}
 	if inboundBufferSize == 0 {
+		closeUnicastConns(unicastPktConnV4, unicastPktConnV6)
 		return nil, errNoPositiveMTUFound
 	}
 	if inboundBufferSize > maxPacketSize {
 		inboundBufferSize = maxPacketSize
 	}
 	if joinErrCount >= len(ifaces) {
+		closeUnicastConns(unicastPktConnV4, unicastPktConnV6)
 		return nil, errJoiningMulticastGroup
 	}
 
 	dstAddr4, err := net.ResolveUDPAddr("udp4", destinationAddress4)
 	if err != nil {
+		closeUnicastConns(unicastPktConnV4, unicastPktConnV6)
 		return nil, err
 	}
 
 	dstAddr6, err := net.ResolveUDPAddr("udp6", destinationAddress6)
 	if err != nil {
+		closeUnicastConns(unicastPktConnV4, unicastPktConnV6)
 		return nil, err
 	}
 
 	var localNames []string

+// closeUnicastConns closes whichever of pc4/pc6 is non-nil. Server opens
+// both before every error return below, none of which close them.
+func closeUnicastConns(pc4 *ipv4.PacketConn, pc6 *ipv6.PacketConn) {
+	if pc4 != nil {
+		_ = pc4.Close()
+	}
+	if pc6 != nil {
+		_ = pc6.Close()
+	}
+}
```

Per the no-third-party-PRs rule this is recorded here only; JP decides
whether to send it to pion/mdns.
