---
# gosd-ry3b
title: 'gosd-init: provisioning snapshot in /data + first-boot-after-reflash self-heal'
status: todo
type: feature
priority: normal
created_at: 2026-07-31T09:17:50Z
updated_at: 2026-07-31T09:17:50Z
blocked_by:
    - gosd-acdn
---

Phase 1 of the upgrade-path design (bean gosd-inau, docs/design/upgrade-path.md §3). Snapshot effective gosd.toml + contemporaneous baked [env] defaults + image identity into /data/.gosd/provision-snapshot/ (durable-write rules) after provisioning settles each boot. On first boot after reflash (image identity differs): restore provable hand-edits into the new card's gosd.toml (written back to GOSD-BOOT so the operator can still see/edit), restore hostname/WiFi only when the fresh boot provides none (wizard-skipped flow). Locked invariant: wizard wins over snapshot; snapshot wins over baked defaults. Exact merge rules decided here within that invariant. No data partition = no snapshot = no self-heal (documented).
