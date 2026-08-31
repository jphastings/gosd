---
# gosd-auvn
title: Wire wifiap into StartNetworking + amend the mDNS-only-listener contract + docs
status: todo
type: task
priority: normal
created_at: 2026-08-31T05:36:11Z
updated_at: 2026-08-31T05:36:15Z
parent: gosd-qfbk
blocked_by:
    - gosd-0jl3
---

WiFi AP epic gosd-qfbk bean 5 (needs bean 4's config.json bit + config tree
field names). Blocks bean 6 (bench verification needs the real wired path).

## Locked decisions

- One `guard.Go("wifiap", ...)` inside `main.go`'s `StartNetworking`,
  alongside the existing `cloudflared`/`tsfunnel` blocks.
- **v1: AP and station mode are mutually exclusive** on one physical
  interface (epic decision 4) — if `config/wifi-ap/ssid` is set and hostapd
  was baked, `wifiap.Run` runs instead of `wifiup.Run` on the WiFi
  interface. No virtual-interface/concurrent-mode juggling.
- Same-PR contract amendments (the gosd-oyhi carve-out, extended — the
  precedent is bean gosd-66ax's cloudflared wiring, follow that shape):
  - [ ] CLAUDE.md's "mDNS is the only network listener" locked decision
        gains the amendment bullet recorded verbatim in epic gosd-qfbk's
        body (decision 2) — draft text ready, land it here.
  - [ ] CLAUDE.md's "Third-party binary blobs" bullet gains hostapd joining
        "kernels and U-Boot" as something GoSD compiles and redistributes
        under `artifacts/vX.Y.Z` (epic decision 10).
  - [ ] `docs/runtime.md`'s "small, fixed set of gosd-shipped system
        services" line (~L1127-1132, the `--with-external` section) gains
        `wifiap` alongside cloudflared/tailscale-funnel.
  - [ ] New `docs/wifi-ap.md`, mirroring `docs/externals.md`'s style:
        `--wifi-ap` usage, config tree keys, the isolated-subnet/no-internet
        guarantee and its software-policy-not-kernel-hardening caveat, the
        arm64-only v1 scope, per-board hardware-support caveat.
  - [ ] COMPATIBILITY.md row/footnote (arm64-only, "not yet hardware-
        verified" until bean 6 flips it — mirrors cloudflared's own
        footnote pattern).
- qemu smoke (CI): mirrors bean gosd-66ax's cloudflared precedent —
  `gosd run --wifi-ap` with no config → exactly the "baked but not
  configured" line; real AP establishment needs real WiFi hardware and is
  bench-only (bean 6), do not attempt to fake it in CI.

## Todos

- [ ] `guard.Go("wifiap", ...)` in `StartNetworking` + `wifiapDeps`
      constructor, exclusive-with-STA branch
- [ ] CLAUDE.md: land the mDNS-listener amendment (decision 2's text)
- [ ] CLAUDE.md: third-party-blobs wording update (decision 10)
- [ ] `docs/runtime.md`: supervised-services line update
- [ ] New `docs/wifi-ap.md`
- [ ] COMPATIBILITY.md row
- [ ] qemu-boot CI job: additive `gosd run --wifi-ap` no-config smoke step
