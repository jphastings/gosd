---
# gosd-489p
title: knope strips package.json's trailing newline, failing the oxfmt gate
status: completed
type: bug
priority: normal
created_at: 2026-08-14T09:43:02Z
updated_at: 2026-08-14T09:43:02Z
parent: gosd-vt2l
---

knope 0.23.0's package.json serializer writes the file without a trailing newline, so every npm release PR fails `vp fmt --check` (first seen on release PR #285 after gosd-96qg's change file landed). The version bump itself is correct — the diff is literally one missing byte.

Fix: a `shell = true` Command step in knope.toml's prepare-release workflow, between PrepareRelease and the commit, restoring the newline when absent and re-staging the file. Proven in the spike clone: committed package.json is bumped AND newline-terminated; the guard no-ops when the newline is present.

**Upstream note (JP decides whether to report):** this is a knope bug worth filing at knope-dev/knope — its `package.json` versioned-file writer should preserve the original file's trailing newline. Do NOT open the issue/PR without JP's say-so.

## Todos

- [x] knope.toml: newline-restore Command step (shell = true) after PrepareRelease
- [x] Verify merged: prepare-release refresh re-issued the release PR (knope closed #285 and opened #287) with the newline intact; CI green, no manual action

## Summary of Changes

A guarded `shell = true` Command step in knope.toml's prepare-release workflow restores package.json's trailing newline after PrepareRelease and re-stages the file (PR #286). Verified in production: the next refresh produced release PR #287 with a green js gate. The upstream knope bug remains unreported pending JP's decision.
