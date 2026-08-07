---
# gosd-u2gz
title: 'provsnapshot: classify [ingress.tailscale-funnel] (snapshot whole, like WiFi)'
status: todo
type: task
priority: normal
created_at: 2026-08-07T15:09:44Z
updated_at: 2026-08-07T15:09:47Z
parent: gosd-65uy
blocked_by:
    - gosd-85bn
    - gosd-tgzo
---

Tailscale epic gosd-65uy bean 6 (after TS-1 gosd-85bn; cloudflared's
gosd-tgzo lands the table-driven [ingress.<agent>] classification this bean
extends).

## Locked decisions

- [ingress.tailscale-funnel] snapshots WHOLE-SECTION, like WiFi and
  [ingress.cloudflared] — one new table row + tests, no new classification
  logic. Authkey = operator intent; baked default is always empty in v1, so
  any present section is a hand-edit (= fresh intent).
- Test the LAYERED reflash property explicitly: the toml section is restored
  by the snapshot AND the node identity already lives on /data
  (/data/.gosd/tailscale, epic decision 3) → after a plain Imager reflash of
  a --data-size=expand image the device reconnects as the SAME node with the
  SAME public URL and no re-auth. Assert both layers.

## Todos

[ ] classification table row + tests
[ ] layered reflash-property test
