---
# gosd-1kw1
title: Provisioning snapshot doesn't preserve a hand-edited data_flush across reflash
status: todo
type: task
priority: low
created_at: 2026-08-02T14:25:46Z
updated_at: 2026-08-02T14:25:46Z
---

Found while implementing gosd-9m1k (PR #177). The provisioning snapshot
(gosd-ry3b) classifies and restores hostname/WiFi/[env] hand-edits after
a reflash, but gosd.toml's new data_flush key is not snapshotted — a
hand-edited `data_flush = true` reverts to the baked default on upgrade,
inconsistent with the hand-edits-survive-upgrades story.

Decide: either add DataFlush to the snapshot's per-key classification
(restore iff it differs from the contemporaneous baked default and the
fresh card shows no fresh intent — the existing rule shape), or
explicitly document it as build-flag-owned (upgrade docs note that card-
level data_flush is per-flash). Small either way; the first keeps one
mental model.
