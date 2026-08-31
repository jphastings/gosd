---
# gosd-qfbk
title: 'WiFi Access Point mode: gosd build --wifi-ap (arm64 + armv6)'
status: todo
type: epic
priority: normal
created_at: 2026-08-31T05:34:56Z
updated_at: 2026-08-31T06:38:40Z
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
   wording drafted in bean gosd-auvn. `gosd build --wifi-ap` lets gosd-init
   supervise a GoSD-compiled hostapd binary (the same gosd-oyhi carve-out
   already granted cloudflared/tailscale-funnel) plus an in-process DHCPv4
   server (`insomniacslk/dhcp/dhcpv4/server4`, already a direct dependency,
   unused subpackage). Both bind exclusively to the WiFi interface this
   feature puts into AP mode — never to any interface carrying a route to the
   internet or to another network segment. v1 does NOT enable hostapd's own
   `ctrl_interface` — see decision 12 and follow-up bean gosd-rb4u.
3. **IP range `10.66.0.1/24`**, device at `.1`, DHCP pool `.10`–`.200`
   (pool size confirmed by JP 2026-08-31). Deliberately distinct from bean
   gosd-30jz's planned `10.55.0.1/24` (USB Ethernet gadget) so the two can
   never collide if both are ever baked on one board.
4. **AP and station mode are mutually exclusive on one physical interface in
   v1** (confirmed by JP 2026-08-31). If the AP is enabled, `wifiap.Run` runs
   instead of `wifiup.Run` on the WiFi interface — no virtual-interface/
   concurrent-mode juggling; hostapd puts the physical interface into AP mode
   itself. A dual-Ethernet+WiFi board (e.g. pi-3b) can still host AP-on-WiFi
   while using Ethernet as uplink today without any concurrency work.
5. **Scope: arm64 AND armv6 (pi-zero-w included)** — revised 2026-08-31 from
   an initial arm64-only proposal, which had been borrowed from cloudflared's
   scoping without checking whether the reasoning transferred. It does not:
   cloudflared excludes armv6 because its *official prebuilt* binary is
   GOARM=7 and faults on armv6 (gosd-aur4), whereas GoSD compiles hostapd
   itself at whatever GOARM level it chooses. Nor does pi-zero-w's WiFi bug
   history disqualify it — bean gosd-6nl2 was phantom `mac80211_hwsim`
   interfaces plus a missing `CONFIG_MMC_SDHCI_IPROC`, fixed and
   bench-verified 2026-07-26, and it *proved* the BCM43430 firmware does the
   offloaded WPA2 handshake. BCM43430 + `brcmfmac` + `nl80211` + hostapd is a
   well-documented AP configuration. Known brcmfmac AP-mode caveats to
   respect in the generated `hostapd.conf`: no `ieee80211w`/MFP support in AP
   mode, and 20MHz-only (skip 40MHz `ht_capab`) — neither of which this
   feature needs. Remaining armv6 risk is ordinary build verification, not
   architecture: bean gosd-hawd must prove the cross-compile, bean gosd-k6rr
   must bench both boards.
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
10. **hostapd is BSD-3-clause, NOT GPL — corrected 2026-08-31.** An earlier
    version of this bean called it GPL-2 and planned to treat redistribution
    "the same as the kernel"; that was wrong and the framing is dropped.
    hostapd/wpa_supplicant were dual BSD/GPLv2 historically but the project
    **removed the GPLv2 option on 2012-02-11** and has shipped BSD-3-clause
    only since (upstream `COPYING`: "As of February 11, 2012, the project has
    chosen to use only the BSD license option for future distribution... any
    distribution of this software after February 11, 2012 is no longer under
    the GPL v2 option"; files still carrying GPLv2 pointers keep them "only
    for attribution purposes"). Obligations are therefore attribution-only —
    reproduce the copyright notice, license text and disclaimer with the
    binary; no source-disclosure or copyleft requirement, and nothing that
    touches GoSD's or an app's own licensing. Bundle hostapd's license text
    with the artifact and keep recording source repo/commit/config as good
    practice (reproducibility), not as GPL provenance. **Separate real
    obligation to check in bean gosd-hawd:** hostapd's `nl80211` driver
    normally links `libnl`, which is **LGPL-2.1** — a fully static musl build
    statically links it, and LGPL-2.1 §6 then requires recipients be able to
    relink against a modified libnl. Confirm whether a no-`libnl` build is
    viable; if not, ship libnl's notice + relinkable objects/source
    separately from hostapd's BSD notice. (Report:
    scratchpad `gpl-license-report.md`, 2026-08-31.)
11. **The AP's boot state is build-configurable, and the app can toggle it at
    runtime through a new public Go package** (JP, 2026-08-31). Three parts:
    (a) `gosd build --wifi-ap` accepts a default boot state
    (enabled/disabled) via flag/`gosd.toml`; (b) the SSID defaults to one
    derived from the device hostname when not set via flag/`gosd.toml`/the
    config tree, so a baked image is usable with zero configuration; (c) a
    **new top-level public package** (`wifiap/`, app-facing `Enable()`/
    `Disable()`/`Status()`) lets the app turn the AP on and off
    programmatically. That makes it **semver-relevant public API surface** —
    CLAUDE.md's public-API bullet must gain it (bean gosd-auvn).
    **Mechanism:** the public package writes a desired-state file under
    `/run/gosd/wifiap/`, which gosd-init's supervisor watches and reconciles
    — matching the existing cross-process marker-file idiom
    (`/run/gosd/network-up`, `/run/gosd/time-synced`) and adding no IPC
    socket, no listener, and no new interactive surface. Naming note: the
    public package and gosd-init's internal one would share the short name
    `wifiap` at different import paths; if that proves confusing during
    implementation, rename the *internal* one (it is internal, so renaming
    costs nothing).
12. **No app-visible connected-client list in v1** (confirmed by JP
    2026-08-31), and that is expected to be follow-up work rather than a
    permanent no — tracked as bean **gosd-rb4u**. It needs hostapd's
    `ctrl_interface`, which is a local unix-domain socket (not
    network-reachable) but still a deliberate widening of decision 2's
    narrow amendment, so it gets its own decision rather than riding along.

## Bench status

Nothing is on the bench as of 2026-08-31 (JP) — hardware verification (bean
gosd-k6rr) is expected to happen later. Beans 1-5 are desk/CI work and are
not blocked by this; per this project's convention the epic can close on
shipped, CI-proven code with gosd-k6rr still open and re-parented.

## Child-bean order

hostapd build rail → dhcpserver package → wifiap package (+ public control
package) → build-time wiring → main.go wiring + CLAUDE.md amendment + docs →
bench verification. Follow-up (not blocking): gosd-rb4u, connected-client
visibility.
