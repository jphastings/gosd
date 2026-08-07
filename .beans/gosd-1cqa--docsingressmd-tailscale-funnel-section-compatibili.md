---
# gosd-1cqa
title: docs/ingress.md tailscale-funnel section + COMPATIBILITY.md row
status: todo
type: task
priority: normal
created_at: 2026-08-07T15:10:02Z
updated_at: 2026-08-07T15:10:06Z
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

[ ] docs/ingress.md tailscale-funnel section
[ ] COMPATIBILITY.md row + footnotes
