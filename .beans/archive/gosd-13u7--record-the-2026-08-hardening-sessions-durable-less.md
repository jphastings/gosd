---
# gosd-13u7
title: Record the 2026-08 hardening session's durable lessons in CLAUDE.md
status: completed
type: task
priority: normal
created_at: 2026-08-03T18:30:55Z
updated_at: 2026-08-03T18:32:28Z
---

Wrap-up of the review-sweep + upgrade-path session: fold the lessons future agents need into CLAUDE.md — the CONFLICTING-PR-silent-CI trap, foreground gates (background monitors stall agents), shared-GOCACHE corruption symptoms, the data-safety commit-marker/adversarial-review rule, the go-diskfs sharp edges (route FAT32 through internal/diskfmt), and the layout-ABI + flush-default locked decisions from docs/design/upgrade-path.md and gosd-9m1k.



## Summary of Changes

CLAUDE.md gained: the CONFLICTING-PR-silent-CI trap (Workflow); foreground
gates/CI-polling and shared-GOCACHE corruption symptoms (Quality gates);
the data-safety crash-ordering + adversarial-review rule with the
write→sync→marker→sync pattern, and the go-diskfs route-through-diskfmt
rule (Code conventions); the layout-ABI and flush-default locked
decisions (Locked decisions). Filed alongside: gosd-e721 (pending
upstream patches for JP to send) and gosd-ftw7 (bench pass hardware-
verifying the 2026-08 runtime hardening batch).
