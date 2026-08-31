---
# gosd-k6rr
title: 'Bench verification: WiFi AP mode on real hardware (pi-zero-2w first)'
status: todo
type: task
priority: normal
created_at: 2026-08-31T05:36:11Z
updated_at: 2026-08-31T05:36:15Z
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

`qemu-virt` has no WiFi hardware, so nothing before this bean can prove the
real AP path. Target pi-zero-2w first — gosd-30jz's own reference board and
the fleet's most likely brcmfmac AP-capable chip.

## Locked decisions

- AP-mode support is chip/driver-dependent and **not guaranteed on every
  board** — "not supported on this board" is a legitimate outcome here, not
  just a delay. Record whichever result actually happens.
- The pass must confirm, on real hardware, all of:
  - [ ] A real phone/laptop associates over WPA2-PSK
  - [ ] It gets a real DHCP lease from `dhcpserver`
  - [ ] `<hostname>.local` resolves from that client
  - [ ] Critically: the client genuinely CANNOT reach anything outside
        `10.66.0.0/24` from that connection (the "no internet" requirement —
        don't just assert it, prove it, e.g. attempt a request to a known
        external host and confirm it fails/times out)
  - [ ] `/proc/sys/net/ipv4/ip_forward` stayed `0` throughout
- One bench bean per board attempted, starting with pi-zero-2w; repeat for
  other arm64 boards as bandwidth allows. Record per-board pass/fail in
  COMPATIBILITY.md (bean 5's row) and in this bean's body.

## Todos

- [ ] pi-zero-2w: full bench pass (association, DHCP, mDNS, isolation,
      ip_forward check) — record result
- [ ] COMPATIBILITY.md: flip the "not yet hardware-verified" footnote once
      pi-zero-2w passes
- [ ] Follow-up beans for additional arm64 boards, lower priority, opened
      individually rather than blocking this one
