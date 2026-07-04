---
# gosd-1epa
title: Supervisor probation mode + three-rung rollback ladder
status: todo
type: task
priority: deferred
created_at: 2026-07-04T21:04:04Z
updated_at: 2026-07-04T21:04:04Z
parent: gosd-vxal
blocked_by:
    - gosd-6k2n
---

Extend cmd/gosd-init/internal/boot supervisor: newly-activated slot must run stably for the defined probation window before being marked good; failures fall new slot → previous good → baked factory /app. Probation must END (defined in the doc). Includes the read-write remount window for slot.state updates.
