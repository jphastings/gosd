---
# gosd-522n
title: 'Phase-2 design: self-update of boot files over the network (+ sneakernet bundle)'
status: todo
type: feature
priority: normal
created_at: 2026-07-31T09:17:50Z
updated_at: 2026-07-31T09:17:50Z
blocked_by:
    - gosd-vxal
---

Phase 2 of the upgrade-path design (bean gosd-inau, docs/design/upgrade-path.md §6). Design (not build): staged download of a boot-file payload to GOSD-BOOT, verify-then-commit, the manifest-of-owned-paths deletion scheme (§5), catalog extract_sha256 polling for update discovery, and the sneakernet bundle (route 4) as the offline carrier of the same payload format. Rides gosd-vxal's endpoint/auth (per-image HMAC). Bootloader stays pinned (locked, §0).
