---
# gosd-odx3
title: Auto-open the internal/artifacts.Version pin-bump PR after an artifacts release
status: todo
type: feature
priority: normal
created_at: 2026-08-14T06:00:53Z
updated_at: 2026-08-14T06:53:06Z
parent: gosd-vt2l
blocked_by:
    - gosd-gnnn
---

JP chose auto-open (2026-08-13). Lands after the first knope artifacts release validates the flow.

## Todos

- [ ] Final job on build-artifacts.yml after asset upload (tag runs only): checkout main; sed `const Version = "v…"` in internal/artifacts/artifacts.go to `${GITHUB_REF_NAME#artifacts/}`; splice the newest docs/releases/artifacts.md section into the doc-comment mini-changelog; `gh pr create` via an app installation token (NOT the default GITHUB_TOKEN — a GITHUB_TOKEN-created PR gets no CI). Since this job runs on a tag push, minting from the knope-release environment needs its deployment policy extended to allow `artifacts/v*` tags — decide then whether to extend it or add a sibling environment
- [ ] PR body names the three-way verification (clean-machine build, offline re-run, dtb spot-check) as the human gate; curated-prose polish happens in review
- [ ] docs/artifacts.md: pin-bump steps now start from the auto-opened PR
