---
# gosd-w2vk
title: 'CLAUDE.md: refresh stale facts + encode build-kernel/beans-CLI operational lessons'
status: completed
type: task
priority: normal
created_at: 2026-07-13T06:39:42Z
updated_at: 2026-07-13T06:49:10Z
---

Post-epic sweep (JP-approved, 2026-07-13): five small updates keeping CLAUDE.md honest after the build-kernel epic ([[gosd-47rm]]), the sattrack example ([[gosd-e9fy]]), and this session's field lessons.

## Summary of Changes

Five CLAUDE.md updates: intro points at COMPATIBILITY.md instead of naming two
boards; Board IDs lose their point-in-time status qualifiers; the Workflow
section documents `beans create --json`'s `.bean.id` path next to the
existing `--body-replace` quirk; Board work gains the build-kernel
cache/staging bullet (content-addressed cache, home-dir bind-mount constraint
from gosd-0p21/gosd-l4y9, colima support); the examples convention points at
examples/sattrack as the big-example reference. Also files [[gosd-6zd1]]
(HTTPS/CA-roots documentation task) in the same PR — filed, not implemented.

## Todos

- [x] Intro: stop naming two boards; point at COMPATIBILITY.md
- [x] Board IDs: drop churny status qualifiers, keep epic refs
- [x] Workflow: document beans create --json id at .bean.id
- [x] Board work: build-kernel cache semantics + bind-mount staging constraints ([[gosd-0p21]]/[[gosd-l4y9]]) + colima
- [x] Examples: point at examples/sattrack as the display/custom-kernel reference
- [x] Quality gates
