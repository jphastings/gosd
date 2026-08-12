---
# gosd-akk4
title: 'netup/wifiup: one shared /run/gosd/network-up marker and resolv.conf with no arbitration — either medium going down clears the other''s state'
status: completed
type: bug
priority: normal
created_at: 2026-07-31T07:52:39Z
updated_at: 2026-07-31T07:52:39Z
---

Found by review sweep `gosd-fuxs` (gosd-init runtime area), verified.

netup's link-down branch (netup.go:150) and wifiup's association-loss path
(wifiup.go:335) both call the same `ClearNetworkUp` closure targeting
`netup.DefaultNetworkUpPath` (wired in cmd/gosd-init/main.go) — a single
boolean file with no refcount or per-interface state. Same aliasing for
/etc/resolv.conf and the default route: two independent state machines
overwrite each other on every lease.

**Failure scenario:** a pi-3b boots with Ethernet and WiFi both up.
Unplugging the cable deletes the marker even though wlan0 holds a valid
lease and route; an app polling the documented marker (docs/runtime.md)
concludes it is offline until wlan0's next DHCP renewal — potentially
hours. DNS/default-route flap the same way in lease order.

**Fix:** refcount the marker in the wiring layer (shared upSet keyed by
interface; write marker when non-empty, remove when empty); same shape for
resolv.conf (keep the last still-up interface's DNS rather than blindly
clearing/overwriting).

## Summary of Changes

Added `netup.UpSet`, a mutex-guarded per-interface refcount over the
shared /run/gosd/network-up marker: mark fires only on the
empty→non-empty transition, clear only on non-empty→empty, repeats are
no-ops. `netup.Deps` and `wifiup.Deps`' MarkNetworkUp/ClearNetworkUp
became `func(iface string) error`, keyed by ev.Name / ifi.Name at every
call site, and main.go wires both packages through one UpSet instance.
The mDNS `Changed` notification deliberately still fires on every
link/lease event regardless of the refcount decision, preserving the
gosd-r796 restart-on-every-lease behavior. DNS/default-route arbitration
was scoped out (resolv.conf handling changed in parallel under
gosd-s2yu; a joint arbitration story can be a follow-up if dual-homed
DNS flapping shows up in practice). Tests: UpSet unit coverage
(transitions, repeats, error propagation) plus a wiring-level
dual-interface test in wifiup proving eth-down keeps the marker while
wlan is up, both-down removes it, and a returning interface recreates
it. (Finished by the coordinating session after two infrastructure
stalls; all gates re-run clean from this worktree.)
