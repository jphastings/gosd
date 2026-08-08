---
# gosd-2194
title: 'Cut the next CLI release: ext4-by-default breaking change ships'
status: completed
type: task
priority: normal
created_at: 2026-08-07T19:04:27Z
updated_at: 2026-08-08T04:17:41Z
---

The disk/ ext4 default (epic gosd-lfu0, merged PRs #186/#187/#192/#194) is a breaking behavior change gated on a CLI release. docs/releases/UNRELEASED.md holds the drafted note. Procedure: promote UNRELEASED.md's content into the tag's release notes, JP pushes the next minor vX.Y.Z CLI tag (CLI tags are plain semver, artifacts tags are the separate artifacts/ namespace — the 2026-08-07 v0.9.0/artifacts-v0.9.0 mixup is the cautionary tale), verify the released gosd builds images that pin artifacts v0.9.0. Also decide whether the release notes should mention cubie-a5e's activation (first Allwinner board) — it shipped in the same window.

## Summary of Changes

v0.1.0 tagged by JP (annotated, at ec9abad) and published: https://github.com/jphastings/gosd/releases/tag/v0.1.0 — the first CLI release. Notes distilled from docs/releases/UNRELEASED.md with two corrections the file predated: Pi kernels DO carry ext4 as of artifacts v0.10.0 (the release this CLI tag pins), and cubie-a5e is public, not internal-only. Version chosen: v0.1.0 (first-ever CLI tag; the CLI/artifacts/npm tracks stay deliberately independent — the 2026-08-07 v0.9.0-vs-artifacts/v0.9.0 mixup is why this bean spells the namespaces out). UNRELEASED.md reset to an empty stub recording the fold. The cubie-a5e activation note rides in "Also in this release".
