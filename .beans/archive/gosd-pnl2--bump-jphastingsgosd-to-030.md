---
# gosd-pnl2
title: Bump @jphastings/gosd to 0.3.0
status: completed
type: task
priority: normal
created_at: 2026-08-13T20:41:58Z
updated_at: 2026-08-13T20:42:23Z
---

Step 1 of the npm release procedure for the config-tree client (epic gosd-rw6n): the publish workflow refuses a tag whose version disagrees with the manifest, so this bump lands before the npm/gosd/v0.3.0 tag. Minor rather than patch per the epic: the config option replaces the env option wholesale — breaking relative to 0.2.0, but 0.2.0 only ever reached the next dist-tag, never latest.

## Summary of Changes

js/packages/gosd/package.json version 0.2.0 → 0.3.0, plus a factual
correction appended to epic gosd-rw6n (v0.5.0 and npm 0.2.0 had shipped,
contrary to the epic's premise). After this merges: tag npm/gosd/v0.3.0 on
the merge commit → CI publishes to `next` (npm-publish environment approval
required) → JP promotes to `latest`, leapfrogging 0.2.0 by design.
