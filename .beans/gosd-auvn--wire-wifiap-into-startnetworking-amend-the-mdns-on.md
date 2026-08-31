---
# gosd-auvn
title: Wire wifiap into StartNetworking + amend the mDNS-only-listener contract + docs
status: todo
type: task
priority: normal
created_at: 2026-08-31T05:36:11Z
updated_at: 2026-08-31T06:40:17Z
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
  interface (epic decision 4) — when the AP is enabled, `wifiap.Run` runs
  instead of `wifiup.Run` on the WiFi interface. No virtual-interface/
  concurrent-mode juggling. Note the AP can be toggled at runtime by the app
  (epic decision 11), so this is a live either/or the supervisor reconciles,
  not a boot-time-only branch.
- Same-PR contract amendments (the gosd-oyhi carve-out, extended — the
  precedent is bean gosd-66ax's cloudflared wiring, follow that shape):
  - [ ] CLAUDE.md's "mDNS is the only network listener" locked decision
        gains the amendment bullet recorded in epic gosd-qfbk (decision 2).
        Draft text is in the plan record
        (`/Users/jp/.claude/plans/is-it-possible-for-cozy-lantern.md`);
        adjust its closing clause to match what actually shipped.
  - [ ] CLAUDE.md's **public API surface** bullet gains the new top-level
        `wifiap/` package (epic decision 11) alongside `gadget/`, `emmc/`,
        `disk/`, `sound/`, `fault/` — it is semver-relevant.
  - [ ] CLAUDE.md's "Third-party binary blobs" bullet: hostapd is compiled
        by GoSD and shipped in `artifacts/vX.Y.Z` alongside kernels and
        U-Boot. **Do not describe it as GPL** — it is BSD-3-clause (epic
        decision 10); the existing bullet's GPL-compliance parenthetical
        applies to the kernel/U-Boot, not to hostapd.
  - [ ] `docs/runtime.md`'s "small, fixed set of gosd-shipped system
        services" line (~L1127-1132, the `--with-external` section) gains
        `wifiap` alongside cloudflared/tailscale-funnel.
  - [ ] New `docs/wifi-ap.md`, mirroring `docs/externals.md`'s style:
        `--wifi-ap` usage, the build-configurable boot state and
        hostname-derived SSID default, the public `wifiap` package's
        `Enable`/`Disable`/`Status`, config tree keys, the isolated-subnet/
        no-internet guarantee and its software-policy-not-kernel-hardening
        caveat, and the per-board hardware-support caveat.
  - [ ] COMPATIBILITY.md row/footnote ("not yet hardware-verified" until
        bean 6 flips it — mirrors cloudflared's own footnote pattern).
- qemu smoke (CI): mirrors bean gosd-66ax's cloudflared precedent —
  `gosd run --wifi-ap` exercises the config/plumbing path; real AP
  establishment needs real WiFi hardware and is bench-only (bean 6), do not
  attempt to fake it in CI.

## Todos

- [ ] `guard.Go("wifiap", ...)` in `StartNetworking` + `wifiapDeps`
      constructor, exclusive-with-STA reconciliation
- [ ] CLAUDE.md: the mDNS-listener amendment (decision 2)
- [ ] CLAUDE.md: public API surface gains `wifiap/` (decision 11)
- [ ] CLAUDE.md: third-party-blobs wording (hostapd, BSD not GPL)
- [ ] `docs/runtime.md`: supervised-services line update
- [ ] New `docs/wifi-ap.md`
- [ ] COMPATIBILITY.md row
- [ ] qemu-boot CI job: additive `gosd run --wifi-ap` smoke step
