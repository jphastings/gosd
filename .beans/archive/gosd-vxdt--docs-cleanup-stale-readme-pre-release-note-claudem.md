---
# gosd-vxdt
title: 'Docs cleanup: stale README pre-release note + CLAUDE.md release bullets'
status: completed
type: task
created_at: 2026-08-14T06:00:53Z
updated_at: 2026-08-14T06:00:53Z
parent: gosd-vt2l
blocked_by:
    - gosd-9qb0
---

## Todos

- [x] README.md: delete the stale "**Pre-release:** no numbered CLI release has been tagged yet" block (v0.5.0 exists); one line pointing at GitHub releases / CHANGELOG.md
- [x] CLAUDE.md release bullets: CLI GitHub releases now live on gosd/vX.Y.Z tags with the plain vX.Y.Z Go-module tag alongside; tags come from the release-PR merge; tag-first/bump-second wording updated to start from the auto-opened pin-bump PR

## Summary of Changes

- `README.md`: removed the stale "Pre-release" callout under Quickstart
  (it claimed no numbered CLI release had been tagged yet — v0.6.0 has
  shipped since). Replaced it with a single line linking to the project's
  GitHub releases page.
- `CLAUDE.md`, "Third-party binary blobs" bullet: "CLI releases are plain
  `vX.Y.Z` tags and pin an artifact version" now describes the knope flow —
  CLI releases are cut by merging the knope release PR, which produces a
  `gosd/vX.Y.Z` GitHub release plus the plain `vX.Y.Z` Go-module tag, and
  pin an artifact version.
- `CLAUDE.md`, "Artifact releases are tag-first, bump-second" bullet: kept
  the tag-first/bump-second principle and the
  don't-bump-`internal/artifacts.Version`-in-the-same-PR/qemu-CI-red
  warning, but replaced the "JP pushes the tag; then a separate follow-up
  PR bumps `Version`" mechanics with the actual knope flow: the artifacts
  change lands with an `artifacts:` change file, merging the knope release
  PR (once it lists `artifacts`) creates the tag and release, and the
  follow-up Version-bump PR proceeds as before. Kept the
  `docs/artifacts.md` pointer and the "Releases are cheap" sentence
  unchanged.
- Checked the rest of `CLAUDE.md` for other now-false release statements
  (searched for `UNRELEASED`, "no automation pushes tags", "hand-push",
  "JP pushes"/"tags"/"release" generally): none found. The "Workflow"
  section's change-file/knope-release-PR bullet (added by the gosd-9qb0
  knope-adoption PR) was already consistent, and `docs/releases/UNRELEASED.md`
  is already gone — no other CLAUDE.md text referenced it.
