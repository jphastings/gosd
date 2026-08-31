---
# gosd-qfbk
title: 'WiFi Access Point mode: gosd build --wifi-ap (arm64 v1)'
status: todo
type: epic
created_at: 2026-08-31T05:34:56Z
updated_at: 2026-08-31T05:34:56Z
---

JP request (2026-08-31, planning session): a gosd device should be able to
host its own WiFi network — AP mode, WPA2-PSK — DHCP-serve clients that join
it, keep that subnet isolated from the internet, and resolve
`<hostname>.local` for those clients, so the app inside sees what looks like
an ordinary, if internet-less, network. Full investigated design in the child
beans; plan session record:
`/Users/jp/.claude/plans/is-it-possible-for-cozy-lantern.md`.

## Locked decisions (JP, 2026-08-31 planning session)

1. **hostapd, cross-compiled by gosd and supervised as a subprocess** — not a
   from-scratch pure-Go nl80211 AP implementation. AP mode means originating
   beacons and running the 4-way handshake as *authenticator* (GTK
   generation/rotation, per-station replay counters — the exact bug class
   behind KRACK) plus a station table — a large, security-sensitive
   undertaking with no existing library or test harness in this codebase to
   build on. hostapd is the mature, audited reference implementation.
   Userspace ELF, not a `.ko`, so the no-loadable-modules decision (gosd-2k9p)
   doesn't apply; the pure-Go/`CGO_ENABLED=0` rule is scoped to gosd's own
   compiled binaries, and a supervised third-party binary is already a
   sanctioned shape (cloudflared, tailscale-funnel, epic gosd-oyhi's
   carve-out). gosd-init's own new code stays small: hostapd does the
   802.11/crypto work; gosd-init brings the interface up and assigns a static
   address (already exposed by `netup.Links` — no new netlink code needed),
   renders `hostapd.conf`, supervises the process (mirrors
   `cmd/gosd-init/internal/cloudflared/cloudflared.go`'s `Deps`/`supervise`/
   `runOnce`/backoff/logwriter shape), and runs the DHCP server.
2. **This AMENDS the "mDNS is the only network listener in gosd-init" locked
   decision** (does not overturn it) — see the exact proposed CLAUDE.md
   wording in bean 5. `gosd build --wifi-ap` lets gosd-init supervise a
   GoSD-compiled hostapd binary (the same gosd-oyhi carve-out already granted
   cloudflared/tailscale-funnel) plus an in-process DHCPv4 server
   (`insomniacslk/dhcp/dhcpv4/server4`, already a direct dependency, unused
   subpackage). Both bind exclusively to the WiFi interface this feature puts
   into AP mode — never to any interface carrying a route to the internet or
   to another network segment. v1 does NOT enable hostapd's own
   `ctrl_interface` (a local unix socket, not network-reachable) — gosd-init
   only start/stop/restart-supervises the process, no IPC needed — so the
   only new listener surface this amendment authorizes is the DHCP server.
3. **IP range `10.66.0.1/24`**, device at `.1`, DHCP pool `.10`–`.200`.
   Deliberately distinct from bean gosd-30jz's planned `10.55.0.1/24` (USB
   Ethernet gadget) so the two can never collide if both are ever baked on
   one board.
4. **AP and station mode are mutually exclusive on one physical interface in
   v1.** If `config/wifi-ap/ssid` is set and hostapd was baked, `wifiap.Run`
   runs instead of `wifiup.Run` on the WiFi interface — no virtual-interface/
   concurrent-mode juggling; hostapd puts the physical interface into AP mode
   itself. A dual-Ethernet+WiFi board (e.g. pi-3b) can still host AP-on-WiFi
   while using Ethernet as uplink today without any concurrency work.
5. **v1 scope: arm64 boards only**, mirroring cloudflared's own initial
   scoping and sidestepping armv6/pi-zero-w's existing WiFi bug history
   (gosd-6nl2).
6. **`dhcpserver` (new package) is built generically** — range-based, not
   single-lease — so bean gosd-30jz can reuse it later as a degenerate
   one-address case. This epic does not wire gosd-30jz itself.
7. **`mdnsresponder` needs zero code changes.** It already re-enumerates
   every `FlagUp` interface on each `Changed` signal; `wifiap.Run` just needs
   to call the same shared `netup.UpSet`/`mdnsresponder.Signal` that
   `netup`/`wifiup` already thread through `main.go`.
8. **No-forwarding guarantee is a software-policy fact, not kernel
   hardening** — nothing in this codebase touches `ip_forward`/`iptables`/
   NAT anywhere today, but the app runs as root and could flip it itself.
   `wifiap` reads `/proc/sys/net/ipv4/ip_forward` at start and logs a loud
   warning if it's already `1` — honest telemetry, not an enforcement claim
   gosd-init doesn't have standing to make.
9. **Secret handling mirrors the existing WiFi-STA pattern exactly** — the
   AP passphrase gets a redaction rule alongside `boot/sequence.go`'s
   existing one; SSID stays unredacted (broadcast on purpose), same as STA.
10. **hostapd joins the `artifacts/vX.Y.Z` compiled-and-redistributed
    channel** (like the kernel and U-Boot), not the pinned-URL-and-sha256
    third-party-blob channel — no prebuilt static hostapd exists for these
    targets. Needs a small CLAUDE.md wording update alongside decision 2's
    amendment (bean 5).

## Open questions (defaults chosen above, flagged as negotiable — revisit if
they turn out wrong rather than silently relitigating)

1. AP+STA concurrency: v1 exclusive per decision 4. A WiFi-only board can't
   do both without per-chip concurrent-interface support (unverified).
2. hostapd's GPL-2 license in the `artifacts/vX.Y.Z` redistribution channel —
   architecturally identical to the kernel's already-solved GPL story, but a
   new *kind* of artifact in that channel (not a kernel or bootloader).
3. Default-SSID auto-enable vs. explicit opt-in: defaults to "empty SSID =
   inert" (mirrors STA), to minimize the new listener's exposure window.
4. arm64-only v1 (decision 5) vs. including armv6 from day one.
5. Per-board hardware support is genuinely unverified and may simply fail on
   some WiFi chips — unverifiable in CI (qemu-virt has no WiFi hardware),
   bench-only; "not supported on this board" is a legitimate outcome.
6. DHCP pool sizing (`.10`–`.200` proposed) — a genuinely multi-client
   subnet, unlike gosd-30jz's single-host USB case.
7. No app-visible "who's connected" list in v1 (needs hostapd
   `ctrl_interface`, deliberately deferred) — confirm acceptable if the
   product story is a pairing/provisioning flow.

## Child-bean order

hostapd build rail → dhcpserver package → wifiap package → build-time wiring
→ main.go wiring + CLAUDE.md amendment + docs → bench verification.
