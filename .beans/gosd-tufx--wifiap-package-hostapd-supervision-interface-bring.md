---
# gosd-tufx
title: 'wifiap: gosd-init supervisor + public app-facing control package'
status: todo
type: task
priority: normal
created_at: 2026-08-31T05:36:11Z
updated_at: 2026-08-31T06:40:17Z
parent: gosd-qfbk
blocked_by:
    - gosd-3ye3
---

WiFi AP epic gosd-qfbk bean 3 (needs bean 2's `dhcpserver` package). Blocks
bean 4 (build-time wiring needs config.json/hostapd.conf field names this
bean settles).

Two packages land here: gosd-init's internal supervisor, and the **new
public app-facing control package** (epic decision 11).

## Locked decisions — gosd-init's supervisor

- New package `cmd/gosd-init/internal/wifiap/`, shaped like `wifiup`/
  `cloudflared` combined — NOT folded into `wifiup` itself (AP and station
  are distinct lifecycles, and `wifiup` is already large and bug-hardened;
  a new guarded goroutine in `main.go`'s `StartNetworking` keeps the
  existing pattern intact — wiring itself is bean 5).
- Interface bring-up reuses `netup.Links.SetUp`/`AddAddr`/`FlushAddrs`
  directly — the exact same interface `netup.Links` already exposes to
  `wifiup`. **No new netlink/genetlink code for L3 addressing.**
- `hostapd.conf` rendering (SSID, WPA2 passphrase or open, channel,
  interface name) to `/run/gosd/wifiap/hostapd.conf` (tmpfs, 0600),
  mirroring `cloudflared.RuntimeDir`/`ConfigPath` exactly. **brcmfmac AP-mode
  caveats to respect in the generated config** (epic decision 5): no
  `ieee80211w`/MFP in AP mode, and 20MHz only — do not emit 40MHz
  `ht_capab`. Neither limits this feature.
- Subprocess supervision: `Deps{StartProcess, Wait, ...}` +
  `supervise`/`runOnce` copied in shape from
  `cmd/gosd-init/internal/cloudflared/cloudflared.go` — same backoff, same
  logwriter prefixing (`"wifiap: "`/`"hostapd: "`), same PID-1-safe Wait
  (never `exec.Cmd.Wait` directly, per cloudflared's own documented
  hazard).
- Composes bean 2's `dhcpserver` once the interface has its static address
  (`10.66.0.1/24`, pool `.10`–`.200` — epic decision 3).
- Calls the *same* shared `netup.UpSet.Up`/`Down` and the *same*
  `mdnsresponder.Signal.Notify()` main.go already wires netup/wifiup
  through. Signals "up" once the interface has its address and the DHCP
  server is bound — NOT gated on any client ever joining, since gosd-init
  is the DHCP *server* here. **No `mdnsresponder` code changes.**
- No-forwarding guard: reads `/proc/sys/net/ipv4/ip_forward` at start, logs
  a loud actionable warning if already `1` (epic decision 8 — honest
  telemetry, not an enforcement claim).
- `platform_linux.go`/`platform_other.go` split only where genuinely needed
  (starting the real process) — most of this package needs no Linux-only
  code, since `netup.Links` and `dhcpserver`'s Deps already carry that seam.
- Fake-driven unit tests need NO dependency on bean 1's real hostapd
  binary — they test process-supervision/signaling logic against a fake
  `StartProcess`.

## Locked decisions — the public control package (epic decision 11)

- **New top-level public package `wifiap/`** — app-facing `Enable()`,
  `Disable()`, `Status()`. This is **semver-relevant public API surface**
  alongside `gadget/`, `emmc/`, `disk/`, `sound/`, `fault/`; CLAUDE.md's
  public-API bullet gains it in bean 5.
- **Mechanism: a desired-state file under `/run/gosd/wifiap/`**, written by
  the public package and watched/reconciled by the supervisor above. This
  matches the existing cross-process marker-file idiom
  (`/run/gosd/network-up`, `/run/gosd/time-synced`) and deliberately adds no
  IPC socket, no listener, and no new interactive surface — so it needs
  nothing beyond epic decision 2's amendment.
- The supervisor reconciles to the desired state: enable → bring the
  interface up, start hostapd + DHCP, signal up; disable → stop hostapd and
  the DHCP server, release the address, signal down. Toggling must be
  idempotent and survive being called repeatedly.
- Off-device (no `gosd` build tag), the package should degrade honestly
  rather than pretend — follow `fault/`'s precedent of a real,
  non-device behaviour instead of a silent no-op.
- Naming note: the public package and the internal one share the short name
  `wifiap` at different import paths. If that proves confusing while
  implementing, rename the *internal* one — it's internal, so renaming is
  free.

## Todos

- [ ] `cmd/gosd-init/internal/wifiap`: interface bring-up, hostapd.conf
      rendering (brcmfmac caveats respected), subprocess supervision
- [ ] Compose `dhcpserver` once the interface has its address
- [ ] Wire the shared `netup.UpSet`/`mdnsresponder.Signal` calls
- [ ] `/proc/sys/net/ipv4/ip_forward` guard + warning log
- [ ] Public `wifiap/` package: `Enable`/`Disable`/`Status` + desired-state
      file, with docstrings (it's public API)
- [ ] Supervisor watches and reconciles the desired-state file, idempotently
- [ ] Fake-driven unit tests (bring-up sequencing, supervision/backoff,
      signaling, enable/disable reconciliation) passing on macOS
