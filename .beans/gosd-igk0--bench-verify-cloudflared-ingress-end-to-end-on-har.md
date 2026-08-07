---
# gosd-igk0
title: 'Bench: verify cloudflared ingress end-to-end on hardware (sdwire, real zone)'
status: todo
type: task
priority: normal
created_at: 2026-08-07T12:52:39Z
updated_at: 2026-08-07T12:53:22Z
parent: gosd-virc
blocked_by:
    - gosd-66ax
    - gosd-tgzo
    - gosd-d1c2
---

Ingress epic gosd-virc final bean. sdwire rig, pi-zero-2w (or any arm64 board),
against a real Cloudflare zone.

## Verify (record results here)

[ ] Token-derived credentials (decoded a/s/t → synthesized credentials.json)
    authenticate for a CLI-created tunnel; public hostname serves the app port
[ ] Characterize a dashboard-created (remote-managed) tunnel run the same way —
    expected: remote config overrides/ignores local ingress; capture the exact
    behavior + log lines for docs
[ ] Reflash survival: snapshot restores the section; tunnel reconnects without
    touching the card
[ ] Cold boot with no network → parks quietly; network arrives → tunnel up
[ ] Serial console log volume acceptable at 115200 with --loglevel warn
[ ] Flip the COMPATIBILITY.md "not yet hardware-verified" footnote
