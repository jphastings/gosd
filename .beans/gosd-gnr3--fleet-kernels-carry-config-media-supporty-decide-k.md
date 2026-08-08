---
# gosd-gnr3
title: Fleet kernels carry CONFIG_MEDIA_SUPPORT=y — decide keep or trim
status: todo
type: task
priority: low
created_at: 2026-08-08T03:45:52Z
updated_at: 2026-08-08T03:45:52Z
---

Split out of gosd-10fn at its close-out (btrfs shipped in artifacts/v0.10.0; this question remains). Every board's kernel (fleet AND Pi) builds the media subsystem in via defconfig inheritance; nothing in gosd uses it, but future camera/capture features might. Decide keep-or-trim with JP; if trim, same shape as the btrfs pass (fragments + qemu ForbiddenY) riding a natural release.
