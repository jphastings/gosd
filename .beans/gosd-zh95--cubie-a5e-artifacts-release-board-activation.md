---
# gosd-zh95
title: 'Cubie A5E: artifacts release + board activation'
status: todo
type: task
priority: normal
created_at: 2026-08-06T22:34:12Z
updated_at: 2026-08-06T22:34:59Z
parent: gosd-h1wv
blocked_by:
    - gosd-o7jv
    - gosd-n6w9
    - gosd-axtv
---

Wire cubie-a5e into .github/workflows/build-artifacts.yml (kernel + uboot jobs mirroring rock-4se's, uploading u-boot-sunxi-with-spl.bin), pre-merge-test the new jobs via workflow_dispatch on the PR branch (the tag run must not be the jobs' first execution), then — tag-first, bump-second — after JP pushes artifacts/vX.Y.Z: a follow-up PR bumps internal/artifacts.Version, flips the board public (Register instead of RegisterInternal) in the SAME activation PR, and updates COMPATIBILITY.md + docs.

## Todos

- [ ] build-artifacts.yml: cubie-a5e-kernel + cubie-a5e-uboot jobs + release-assembly wiring (single u-boot-sunxi-with-spl.bin, not idbloader/itb pair)
- [ ] internal/boards artifacts mapping + tests for the new tarball contents
- [ ] workflow_dispatch pre-merge run green on the PR branch
- [ ] PR 1 (no Version bump) merged; JP pushes artifacts tag
- [ ] Verify the artifact bump three ways (clean-HOME build, offline cache re-run, content spot-check e.g. dtc on the released DTB) — record in this bean
- [ ] PR 2: artifacts.Version bump + public registration + COMPATIBILITY.md/docs/catalog in one activation PR
