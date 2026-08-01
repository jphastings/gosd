---
# gosd-akk4
title: 'netup/wifiup: one shared /run/gosd/network-up marker and resolv.conf with no arbitration — either medium going down clears the other''s state'
status: todo
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
