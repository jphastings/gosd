---
# gosd-vxal
title: App-slot OTA updates (single method, all boards)
status: todo
type: epic
priority: deferred
created_at: 2026-07-04T21:04:04Z
updated_at: 2026-07-04T21:04:04Z
parent: gosd-p3zw
---

Umbrella for the accepted A/B design: OTA updates replace only the APP via slots on GOSD-BOOT; kernel/initramfs/bootloader are reflash-only. Single board-agnostic mechanism per docs/design/ab-updates.md (merged PR #15). The initramfs-baked /app is the immutable factory fallback; rollback ladder: new slot → previous good slot → factory. Do not start before v0.2 ships.
