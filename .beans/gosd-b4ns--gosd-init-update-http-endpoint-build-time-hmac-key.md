---
# gosd-b4ns
title: gosd-init update HTTP endpoint + build-time HMAC key
status: todo
type: task
priority: deferred
created_at: 2026-07-04T21:04:04Z
updated_at: 2026-07-04T21:04:04Z
parent: gosd-vxal
blocked_by:
    - gosd-6k2n
---

The one sanctioned extra listener (CLAUDE.md): GET /update/info, PUT /update, POST /update/activate, HMAC key baked via initcfg at build time, integrity check before activation, concurrent-push rejection, app-size budget enforcement. Per docs/design/ab-updates.md.
