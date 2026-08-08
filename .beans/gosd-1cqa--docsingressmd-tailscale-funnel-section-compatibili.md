---
# gosd-1cqa
title: docs/ingress.md tailscale-funnel section + COMPATIBILITY.md row
status: completed
type: task
priority: normal
created_at: 2026-08-07T15:10:02Z
updated_at: 2026-08-08T06:57:47Z
parent: gosd-65uy
blocked_by:
    - gosd-o68e
    - gosd-d1c2
---

Tailscale epic gosd-65uy bean 7 (after TS-5 gosd-o68e; cloudflared's
gosd-d1c2 creates docs/ingress.md with the "choosing an ingress" overview —
this bean adds the tailscale-funnel section beside cloudflared's).

## Content (locked)

- Overview row: what Funnel is, where TLS terminates (ON-NODE — Let's
  Encrypt via Tailscale ACME; ingress relays route on TLS SNI and cannot
  read traffic), whose account (any Tailscale plan incl. free), board
  support (ALL boards incl. pi-zero-w — contrast cloudflared's arm64-only).
- Runbook: tailnet policy nodeAttr `funnel` + HTTPS certs + MagicDNS
  (admin-console actions the DEVICE cannot do for itself); create a TAGGED,
  REUSABLE auth key (the tag disables node-key expiry — the 180-day default
  would brick an unattended device); paste authkey + port into gosd.toml;
  funnel_port constraint {443, 8443, 10000}; the public URL is
  https://<hostname>.<tailnet>.ts.net.
- Secrets-on-FAT note: authkey sits plaintext on GOSD-BOOT, same trust level
  as the WiFi PSK; only needed for FIRST registration — safe to delete from
  gosd.toml after the device appears in the tailnet.
- Identity/state story: /data/.gosd/tailscale; what survives reboot AND a
  plain Imager reflash (--data-size=expand: same node, same URL, zero
  re-auth); lost-state recovery (new node registers as `hostname-1`,
  CHANGING the public URL — delete the stale node in the admin console and
  provide a live tagged key).
- Key-expiry table: auth key ≤90 days = first-registration-only; tagged
  node key never expires. No-custom-domains and unquantified-bandwidth
  caveats. RAM note: the shim is initramfs-resident; co-baking both ingress
  agents costs roughly 40-60MB RAM (TS-8 measures precisely).
- COMPATIBILITY.md: tailscale-funnel row, all boards ✅ with "not yet
  hardware-verified" footnotes until TS-8 flips them.

## Todos

[x] docs/ingress.md tailscale-funnel section
[x] COMPATIBILITY.md row + footnotes


## Summary of Changes

- `docs/ingress.md`: added the full Tailscale Funnel section (mirroring
  Cloudflare Tunnel's shape) — prerequisites the device can't set for
  itself (MagicDNS, HTTPS Certificates, the `funnel` ACL nodeAttr, with a
  policy-file JSON snippet), the runbook (build with `--data-size`, set up
  the tailnet, create a tagged reusable auth key and why, paste
  authkey+port into `gosd.toml`, delete the key afterwards), a
  `gosd.toml` key reference table (`authkey`/`port`/`hostname`/
  `funnel_port`, defaults, the `{443, 8443, 10000}` set), the data-partition
  hard-error requirement (quoting `cmd/gosd/ingress.go`'s exact refusal
  text), what gets written on the device (argv/env only, no config file,
  `TS_AUTHKEY` never in argv), secrets-on-FAT-partition, clock/TLS and
  restart-backoff behavior, the reflash story (zero re-auth via
  `/data/.gosd/tailscale`, plus the `hostname-1` lost-state recovery path),
  troubleshooting (verbatim log lines from
  `cmd/gosd-init/internal/tsfunnel/mode.go`/`tsfunnel.go` and the shim's own
  wrapped errors from `cmd/gosd-tsfunnel/errors.go`), and caveats (no custom
  domains, bandwidth caps, RAM footprint, not-yet-bench-verified pointing at
  `gosd-79v8`). The "Choosing an ingress" overview table gained the
  Tailscale Funnel row/columns (board support, TLS termination, whose
  account, public URL shape, reflash story) and the shared intro bullets
  were corrected where they assumed a single Cloudflare-edge TLS model.
  `COMPATIBILITY.md`'s row and footnotes were already added by `gosd-kzd3`
  — verified accurate, no changes needed.
- `docs/runtime.md`: generalized the "Ingress" section heading/intro from
  cloudflared-only to cover both agents, fixed the `/data/.gosd/`
  bookkeeping-namespace note to mention the tailscale state directory, and
  updated the provisioning-snapshot reflash prose (both the restore-logic
  list and the "what does not come back" line) to include
  `[ingress.tailscale-funnel]` alongside `[ingress.cloudflared]` — these
  were left stale by `gosd-o68e`, which fixed the other cloudflared-only
  mentions in that file but explicitly deferred `docs/ingress.md`/
  `COMPATIBILITY.md` to this bean.
- Verified against real Tailscale docs (kb/1223 Funnel, kb/1085 auth keys)
  that the ACL nodeAttr JSON shape, the 1-90-day auth key expiry range, and
  "tagging disables node key expiry by default" all match this bean's and
  the epic's locked claims.
