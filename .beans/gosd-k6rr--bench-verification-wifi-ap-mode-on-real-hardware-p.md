---
# gosd-k6rr
title: 'Bench verification: WiFi AP mode on real hardware (pi-zero-2w + pi-zero-w)'
status: todo
type: task
priority: normal
created_at: 2026-08-31T05:36:11Z
updated_at: 2026-08-31T06:40:17Z
parent: gosd-qfbk
blocked_by:
    - gosd-hawd
    - gosd-auvn
---

WiFi AP epic gosd-qfbk bean 6 (needs bean 1's real hostapd binary + bean 5's
runtime wiring shipped on `main`). Sequenced last; per this project's own
convention (see epic gosd-virc/bean gosd-igk0) the epic can close once beans
1-5 have shipped and are CI-proven even while this bean stays open — it then
gets re-parented to no parent, never leaving a completed epic with an open
child.

**Nothing is on the bench as of 2026-08-31 (JP); hardware can be set up
later.** This bean is therefore expected to sit in `todo` for a while — that
is the plan, not neglect. Beans 1-5 are desk/CI work and are not blocked by
it.

`qemu-virt` has no WiFi hardware, so nothing before this bean can prove the
real AP path.

## Locked decisions

- AP-mode support is chip/driver-dependent and **not guaranteed on every
  board** — "not supported on this board" is a legitimate outcome here, not
  just a delay. Record whichever result actually happens.
- **Two boards in scope for v1** (epic decision 5): pi-zero-2w (arm64) and
  pi-zero-w (armv6). Both are BCM43430/43436-class `brcmfmac` radios, the
  best-documented AP-capable parts in the fleet. Take pi-zero-2w first; the
  armv6 leg is the one carrying genuinely new risk (see bean gosd-hawd's
  cross-compile).
- The pass must confirm, on real hardware, all of:
  - [ ] A real phone/laptop associates over WPA2-PSK
  - [ ] It gets a real DHCP lease from `dhcpserver`
  - [ ] `<hostname>.local` resolves from that client
  - [ ] Critically: the client genuinely CANNOT reach anything outside
        `10.66.0.0/24` from that connection (the "no internet" requirement —
        don't just assert it, prove it, e.g. attempt a request to a known
        external host and confirm it fails/times out)
  - [ ] `/proc/sys/net/ipv4/ip_forward` stayed `0` throughout
  - [ ] The app can toggle the AP off and back on via the public `wifiap`
        package (epic decision 11) and clients can rejoin afterwards
  - [ ] The hostname-derived default SSID appears when no SSID was
        configured
- Record per-board pass/fail in COMPATIBILITY.md (bean 5's row) and in this
  bean's body. Additional boards beyond these two get their own beans rather
  than blocking this one.

## Todos

- [ ] pi-zero-2w: full bench pass — record result
- [ ] pi-zero-w (armv6): full bench pass — record result
- [ ] COMPATIBILITY.md: flip the "not yet hardware-verified" footnote per
      board as each passes
- [ ] Follow-up beans for any additional boards, opened individually
