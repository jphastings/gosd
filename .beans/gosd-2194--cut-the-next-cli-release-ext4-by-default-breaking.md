---
# gosd-2194
title: 'Cut the next CLI release: ext4-by-default breaking change ships'
status: todo
type: task
created_at: 2026-08-07T19:04:27Z
updated_at: 2026-08-07T19:04:27Z
---

The disk/ ext4 default (epic gosd-lfu0, merged PRs #186/#187/#192/#194) is a breaking behavior change gated on a CLI release. docs/releases/UNRELEASED.md holds the drafted note. Procedure: promote UNRELEASED.md's content into the tag's release notes, JP pushes the next minor vX.Y.Z CLI tag (CLI tags are plain semver, artifacts tags are the separate artifacts/ namespace — the 2026-08-07 v0.9.0/artifacts-v0.9.0 mixup is the cautionary tale), verify the released gosd builds images that pin artifacts v0.9.0. Also decide whether the release notes should mention cubie-a5e's activation (first Allwinner board) — it shipped in the same window.
