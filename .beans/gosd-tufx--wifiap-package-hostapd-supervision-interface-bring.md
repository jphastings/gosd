---
# gosd-tufx
title: 'wifiap package: hostapd supervision + interface bring-up'
status: todo
type: task
priority: normal
created_at: 2026-08-31T05:36:11Z
updated_at: 2026-08-31T05:36:15Z
parent: gosd-qfbk
blocked_by:
    - gosd-3ye3
---

WiFi AP epic gosd-qfbk bean 3 (needs bean 2's `dhcpserver` package). Blocks
bean 4 (build-time wiring needs config.json/hostapd.conf field names this
bean settles).

## Locked decisions

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
  mirroring `cloudflared.RuntimeDir`/`ConfigPath` exactly.
- Subprocess supervision: `Deps{StartProcess, Wait, ...}` +
  `supervise`/`runOnce` copied in shape from
  `cmd/gosd-init/internal/cloudflared/cloudflared.go` — same backoff, same
  logwriter prefixing (`"wifiap: "`/`"hostapd: "`), same PID-1-safe Wait
  (never `exec.Cmd.Wait` directly, per cloudflared's own documented
  hazard).
- Composes bean 2's `dhcpserver` once the interface has its static address
  (`10.66.0.1/24` — epic decision 3).
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

## Todos

- [ ] `wifiap` package: interface bring-up, hostapd.conf rendering,
      subprocess supervision (mirrors cloudflared.go)
- [ ] Compose `dhcpserver` once the interface has its address
- [ ] Wire the shared `netup.UpSet`/`mdnsresponder.Signal` calls
- [ ] `/proc/sys/net/ipv4/ip_forward` guard + warning log
- [ ] Fake-driven unit tests (bring-up sequencing, supervision/backoff,
      signaling) passing on macOS
