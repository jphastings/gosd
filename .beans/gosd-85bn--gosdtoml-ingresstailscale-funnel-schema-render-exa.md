---
# gosd-85bn
title: 'gosdtoml: [ingress.tailscale-funnel] schema + Render example block'
status: todo
type: task
priority: normal
created_at: 2026-08-07T15:07:49Z
updated_at: 2026-08-07T15:08:08Z
parent: gosd-65uy
blocked_by:
    - gosd-7upw
    - gosd-virc
---

Tailscale epic gosd-65uy bean 1 (after cloudflared's gosd-7upw lands the
Ingress table and Render(Ingress) signature).

## Locked decisions

- Sibling struct in internal/gosdtoml:
  `IngressTailscaleFunnel { Authkey, Hostname string; Port, FunnelPort int }`
  + `Configured() bool` (any field non-zero); field on Ingress is
  `TailscaleFunnel` with toml tag "tailscale-funnel" (TOML bare keys allow
  dashes — no quoting needed in user files).
- Coercion mirrors gosd-7upw exactly: the two strings coerce-with-warning
  from bare scalars; the two ints accept int64 or quoted all-digits;
  warnings name ONLY the key, NEVER the value (authkey is a secret —
  mergeUserEnv precedent); deterministic sorted warning order.
- Semantic validation (port range, funnel_port set membership, hostname
  defaulting) lives in the runtime module, NOT Parse (validHostname
  precedent; matches the cloudflared split).
- Render example block appended after cloudflared's, plain-language per the
  file's voice: authkey from the admin console (tagged + reusable — the tag
  disables node-key expiry), port = the local HTTP service, hostname
  defaults to the device hostname, and the comment states BOTH the
  `gosd build --ingress tailscale-funnel` requirement AND that the authkey
  is only needed for first registration (safe to remove afterwards).

## Todos

[ ] Struct/rawConfig/coercion + warnings, tests (incl. an authkey-never-in-warnings scan)
[ ] Render example block + golden test
