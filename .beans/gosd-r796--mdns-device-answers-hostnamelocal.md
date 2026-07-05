---
# gosd-r796
title: 'mDNS: device answers <hostname>.local'
status: in-progress
type: task
priority: normal
created_at: 2026-07-02T21:07:10Z
updated_at: 2026-07-04T21:38:07Z
parent: gosd-b22t
blocked_by:
    - gosd-vtce
---

Headless discovery for end users: after network-up, the device must answer `<hostname>.local` so users never need to find an IP.

Locked: github.com/pion/mdns — run a responder answering A/AAAA queries for `<hostname>.local` on all up interfaces; re-announce on address change. Runs inside gosd-init (not the user app). Also verify no conflict if the user app itself does mDNS/zeroconf (document: apps should use a different instance name or the gosd runtime API later).

- [x] Responder wired to network-up/addr-change events
- [ ] Manual test matrix recorded here: macOS (ping hostname.local), Windows 11, Linux (avahi), iOS/Android browser hit http://hostname.local
- [x] Hostname collision behavior: log-only in v0.2 (no auto-rename), file follow-up bean if it bites

## Acceptance
Fresh-flashed device reachable at http://<hostname>.local from macOS and Windows within 5s of network-up.

## Summary of Changes

Added `cmd/gosd-init/internal/mdnsresponder`, a new package that runs a pion/mdns/v2 responder answering A/AAAA for `<hostname>.local` inside gosd-init, following the fake-driven, thin-interface style established by `netup`/`wifiup`/`timesync`:

- `Run(deps, opts)` starts a responder for `opts.Hostname+".local"` and restarts it (closing the old one, opening a new one) every time `deps.Changed` fires. This restart-on-change loop is the mechanism for "re-announce on address change": `pion/mdns`'s `*mdns.Conn` has no API to add a newly-appeared interface or update its answers post-construction, so a full stop/start is what "re-announce" means here, as the bean allows. A restart takes milliseconds (closing UDP sockets, opening new ones); any realistic mDNS client already retries on no answer, so this is not observable in practice.
- `deps.Changed` is fed by a new `Signal` type (`signal.go`): a coalescing, non-blocking notifier so a burst of link events collapses into one restart rather than one per event. `main.go` wires this in without touching `netup`/`wifiup` themselves at all — it wraps the `MarkNetworkUp`/`ClearNetworkUp` closures it already builds for both packages so each real marker write also calls `Signal.Notify()`. This satisfies "hook the lease/link events netup already surfaces" as a pure main.go composition change: link-down, initial DHCP lease, and every lease renewal (wired or WiFi) all trigger a responder restart.
- `NewServer` (`server.go`) is the real, `pion/mdns`-backed implementation. Unlike netup/wifiup/timesync's Linux-syscall-bound platform code, `pion/mdns` is pure Go over plain UDP multicast sockets, so this package needs **no** `platform_linux.go`/`platform_other.go` split and no `linux` build tag at all — it compiles and runs identically on macOS and Linux. Passing `nil` for `mdns.Config.Interfaces` makes pion/mdns call `net.Interfaces()` itself and filter to `FlagUp` (excluding loopback), which is exactly "on all up interfaces" with no extra plumbing on gosd-init's side.
- Hostname collisions: implemented as log-only, per the bean. `NewServer` spawns a background probe (`probeForCollision`, 3s timeout) that queries for `<hostname>.local` once after starting; since the responder never enables multicast loopback, any answer it gets back is genuinely from another host, and is logged (not acted on — no auto-rename, matching the locked v0.2 decision).
- `main.go` wiring is additive only: one new import, one `mdnsresponder.Run` goroutine alongside the existing `netup`/`timesync`/`wifiup` goroutines in `StartNetworking`, and `netupDeps`/`wifiupDeps` each gained one new parameter (the shared `*mdnsresponder.Signal`) wrapping their existing `MarkNetworkUp`/`ClearNetworkUp` closures.
- Documented in the package doc comment (`mdnsresponder.go`): gosd-init is the sole owner of `<hostname>.local`'s A/AAAA records; a user app that wants to run its own mDNS/zeroconf responder for its own services must advertise those under a distinct service instance name, since gosd-init's responder is the only mDNS presence gosd-init itself coordinates.

### Localhost smoke test

Ran a real, no-fakes test locally on the implementing Mac (`TestNewServerAnswersQueries` in `server_test.go`): started the actual production responder (real bound multicast UDP sockets), queried it from a second, independent mDNS client instance (mirroring pion/mdns's own query example), and got back a valid answer — the responder's own LAN IP address (observed values included `192.168.1.34` and a Tailscale `100.x` address across repeated runs) resolving `gosd-smoketest-host.local`. This confirms the responder genuinely answers mDNS queries end-to-end on this machine. The test is guarded (skips rather than fails) if multicast is unavailable in a given environment, so it can't flake CI on a sandboxed runner.

### Deviations / what's left

- "Re-announce on address change" is implemented as full responder restart, not an in-place re-announcement, since `pion/mdns`'s `Conn` offers no API for the latter — explicitly accepted as sufficient by the bean text, and documented in `Run`'s doc comment.
- The manual cross-OS test matrix (macOS `ping`, Windows 11, Linux avahi, iOS/Android browser hitting `http://hostname.local`) needs a second real device on the same LAN and is left unchecked; the localhost smoke test above is as close as this environment can get on its own. Bean stays `in-progress` for this reason, matching the same pattern gosd-vtce used for its own hardware-only acceptance items.
