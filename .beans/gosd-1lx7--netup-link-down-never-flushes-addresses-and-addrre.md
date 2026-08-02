---
# gosd-1lx7
title: 'netup: link-down never flushes addresses and AddrReplace only replaces identical prefixes — replug accumulates stale IPs'
status: completed
type: bug
priority: normal
created_at: 2026-07-31T07:53:06Z
updated_at: 2026-08-02T10:35:34Z
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


## Summary of Changes

Re-verified the bug shape against current `netup`/`wifiup` (post-#172/#171/#169):
link-down cancelled DHCP and called the refcounted `ClearNetworkUp(iface)`
but performed no address teardown, and `AddAddr`'s `netlink.AddrReplace`
only replaces an address identical to one already present — so a replug
landing a different lease address left the interface carrying both.

Added `Links.FlushAddrs(name string) error` to the `netup` platform seam
(`cmd/gosd-init/internal/netup/interfaces.go`). `platform_linux.go` uses
the same `github.com/vishvananda/netlink` wrapper AddAddr/AddrReplace
already used (not raw mdlayher/netlink, so the `netlink.Request`-flag
pattern doesn't apply here) via `netlink.AddrList(link, FAMILY_V4)` +
`netlink.AddrDel` per address; `platform_other.go` stubs it out like the
rest of the seam.

Call sites:
- `netup.go`'s link-down branch (`!ev.Up && running`) now calls
  `FlushAddrs(ev.Name)` right after cancelling DHCP, alongside the
  existing refcounted `ClearNetworkUp(ev.Name)` — both keyed by the same
  interface name, so a dual-interface board's other still-up interface
  (gosd-akk4's refcounted marker) is never touched.
- `onLeaseFor` (duplicated identically in `netup` and `wifiup/lease.go`,
  matching the existing mirroring convention) now closes over the address
  it last applied and flushes before `AddAddr` whenever the incoming
  lease's address differs from that — covering both a replug that lands a
  new address and an in-session DHCP restart without a link flap. A
  renewal that keeps the same address is a no-op comparison, so it causes
  no flush and no connectivity blip.
- `wifiup.go`'s `watchAssociation` flushes the interface's addresses on
  confirmed association loss, mirroring the link-down branch (not on the
  `<-stop` graceful-shutdown path, which isn't a loss).

Default-route decision: no explicit route teardown was added. Removing an
interface's addresses is a Linux kernel side effect that already drops any
route depending on them, including a default route whose gateway was only
reachable via the flushed prefix — so flushing correctly clears a stale
default route for free, and a fresh one is set by `ReplaceDefaultRoute`
once the next lease is applied. This is noted in `FlushAddrs`'s doc
comment on the `Links` interface.

Fake fidelity: `fakeLinks` in both `netup` and `wifiup` test packages was
upgraded from a single-address-per-interface map to model real netlink
behavior — `AddAddr` replaces an existing identical address and otherwise
appends (so stale accumulation is actually observable in tests), and
`FlushAddrs` empties an interface's list without touching others', with a
per-interface call counter. `wiringLinks` (wifiup's upset_wiring_test.go)
gained a no-op `FlushAddrs` to keep satisfying `netup.Links`.

New behavioral tests: link-down flushes only that interface's addresses
(dual-interface isolation, gosd-akk4 interplay); a replug landing a new
lease address ends with exactly one (the new) address — the
stale-accumulation regression test; a direct `onLeaseFor` test proves a
same-address renewal doesn't flush while a differing one does exactly
once; wifiup's association-loss test mirrors the flush-then-reconnect
sequence and asserts the same single-address end state.
