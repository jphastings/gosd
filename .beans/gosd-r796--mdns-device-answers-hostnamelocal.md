---
# gosd-r796
title: 'mDNS: device answers <hostname>.local'
status: todo
type: task
priority: normal
created_at: 2026-07-02T21:07:10Z
updated_at: 2026-07-02T21:10:20Z
parent: gosd-b22t
blocked_by:
    - gosd-vtce
---

Headless discovery for end users: after network-up, the device must answer `<hostname>.local` so users never need to find an IP.

Locked: github.com/pion/mdns — run a responder answering A/AAAA queries for `<hostname>.local` on all up interfaces; re-announce on address change. Runs inside gosd-init (not the user app). Also verify no conflict if the user app itself does mDNS/zeroconf (document: apps should use a different instance name or the gosd runtime API later).

- [ ] Responder wired to network-up/addr-change events
- [ ] Manual test matrix recorded here: macOS (ping hostname.local), Windows 11, Linux (avahi), iOS/Android browser hit http://hostname.local
- [ ] Hostname collision behavior: log-only in v0.2 (no auto-rename), file follow-up bean if it bites

## Acceptance
Fresh-flashed device reachable at http://<hostname>.local from macOS and Windows within 5s of network-up.
