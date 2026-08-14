---
# gosd-96qg
title: 'npm release flow: docs + end-to-end validation release'
status: todo
type: task
created_at: 2026-08-14T06:00:53Z
updated_at: 2026-08-14T06:00:53Z
parent: gosd-vt2l
blocked_by:
    - gosd-9qb0
---

publish-npm.yml stays UNTOUCHED (knope tag npm/gosd/vX.Y.Z matches its trigger, dir parse, and version cross-check). This bean aligns the docs and proves the pipeline.

## Todos

- [ ] js/PUBLISHING.md "Cutting a release" rewrite: change file → release PR replaces hand bump-PR + hand tag; environment approval, manual `latest` promotion, and rollback sections unchanged
- [ ] Validate end-to-end with a low-stakes npm patch release: knope (PAT-created) tag → verify job passes (ancestor + version cross-check) → npm-publish environment approval → publishes to `next` → manual promotion. This also proves PAT-created tags trigger workflows BEFORE artifacts relies on the same mechanism
