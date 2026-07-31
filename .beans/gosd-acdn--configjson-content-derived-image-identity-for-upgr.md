---
# gosd-acdn
title: 'config.json: content-derived image identity for upgrade skew detection'
status: todo
type: feature
created_at: 2026-07-31T09:17:50Z
updated_at: 2026-07-31T09:17:50Z
---

Phase 1 of the upgrade-path design (bean gosd-inau, docs/design/upgrade-path.md §4). Add a build-identity field to internal/initcfg's config.json: content-derived (digest over the boot payload set), NOT a timestamp, so identical rebuilds compare equal and qemu CI stays deterministic. Consumers: provisioning-snapshot skew detection, and phase-2 self-update's am-I-already-running-this check (sha-based, no semver).
