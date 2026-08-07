---
# gosd-tgzo
title: 'provsnapshot: classify [ingress.cloudflared] (snapshot whole, like WiFi)'
status: todo
type: task
priority: normal
created_at: 2026-08-07T12:52:39Z
updated_at: 2026-08-07T12:53:22Z
parent: gosd-virc
blocked_by:
    - gosd-7upw
---

Ingress epic gosd-virc bean 5 (needs gosd-7upw). Classify the new section under
provsnapshot's three-way fresh-intent / snapshot-intent / baked-default test
(package doc L1-105).

## Locked decisions

- Snapshot it whole-section, like the WiFi ssid/passphrase pair — never
  field-merged. Pure operator intent; baked default is always empty in v1, so
  any present section is a hand-edit (= fresh intent).
- The token round-trips through /data exactly as WiFi passphrases do. With no
  credentials file on GOSD-BOOT (epic decision 3), ingress fully survives a
  plain Imager reflash — this is a designed property, assert it in tests.
