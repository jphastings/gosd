---
# gosd-8vwy
title: 'Bench verification: runtime WiFi join + ingress reconnect on real hardware'
status: todo
type: task
priority: normal
created_at: 2026-09-01T15:28:18Z
updated_at: 2026-09-01T15:28:18Z
parent: gosd-ojbm
blocked_by:
    - gosd-uy4x
---

Part of the runtime-WiFi-join epic — hardware verification of the shipped code, on JP's bench (sdwire rig). Per the epic-closure convention this bean does not block the epic closing once the code is on main and CI-proven; it gets re-parented at that point.

Verify on real hardware (pi-zero-2w for arm64; pi-zero-w for the armv6 leg):

- Boot an image with NO WiFi configured; a test app calls wifi.Join with real credentials → device associates, gets DHCP, mDNS answers on the new network.
- Persist=true → power cycle → device rejoins by itself. Persist=false → power cycle → device does not.
- With `--ingress cloudflared` (or tailscale-funnel) baked: after a runtime join from a cold no-network boot, the tunnel comes up without any reboot; after a join that *moves* networks, the tunnel comes back.
- Wrong passphrase → Join returns an error naming the reason within the bounded attempt window; device keeps retrying in the background (expected, decision 6).
- Crash-report redaction: force a fault after a runtime join and confirm the runtime passphrase never appears in LAST_FATAL_ERROR.md.

Record surprises in docs/development/<board-id>.md per the board-work convention.
