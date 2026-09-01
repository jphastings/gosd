---
gosd: minor
---

#### `wifi.Join` now works end to end

gosd-init's WiFi bring-up now watches for a runtime `wifi.Join` request and
reconciles it: tears down the current association, attempts the requested
network, and reports back joined or failed with the failure reason as
precisely as nl80211 gives it. This runs on any WiFi-capable board even if
no WiFi credentials were configured at boot, and answers honestly with "no
WiFi interface" on a board with no WiFi hardware at all — an app's `Join`
call never just hangs.

`wifi.Options{Persist: true}` now does something: on a successful join,
gosd-init writes the network into the card's config tree so the device
rejoins it on every future boot too. A failed join is never persisted.

A successful runtime join also restarts the cloudflared or tailscale-funnel
ingress tunnel (whichever the image was built with), so it comes back on
the new network without a reboot — including a join back onto the same
network, which still counts as a fresh restart.
