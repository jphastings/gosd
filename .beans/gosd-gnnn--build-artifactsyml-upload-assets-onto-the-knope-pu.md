---
# gosd-gnnn
title: 'build-artifacts.yml: upload assets onto the knope-published release'
status: in-progress
type: task
priority: normal
created_at: 2026-08-14T06:00:53Z
updated_at: 2026-08-14T06:08:26Z
parent: gosd-vt2l
blocked_by:
    - gosd-dnzo
---

Replace the softprops/action-gh-release "Publish GitHub Release" step (hardcoded static body) with `gh release upload "$GITHUB_REF_NAME" --clobber <explicit dist paths>` onto the release knope creates at release-PR merge. Must land BEFORE any knope artifacts release.

## Todos

- [x] Swap softprops step for gh release upload (explicit per-board paths preserve fail-on-missing; --clobber makes reruns idempotent; existing `permissions: contents: write` suffices)
- [x] workflow_dispatch path untouched (still uploads dist/ as a workflow artifact, no release)
- [x] Update the workflow header comment (knope publishes the release with notes; this workflow attaches assets)
- [x] docs/artifacts.md "Cutting a new release" rewrite: change file + merge the release PR replaces the hand-pushed tag (amends "no automation pushes tags" — the merge is the deliberate act); pin-bump + three-way verification steps stay; document the hand-tag escape hatch (gh release create first)

## Summary of Changes

- `.github/workflows/build-artifacts.yml`: replaced the `softprops/action-gh-release`
  "Publish GitHub Release" step (hardcoded body, `tag_name`/`name`/`files`/
  `fail_on_unmatched_files`) with a "Upload assets to the knope-published release"
  step that runs `gh release upload "$GITHUB_REF_NAME" --clobber` against the
  same explicit list of per-board `.tar.zst` files plus `manifest.json`,
  guarded by the same `if: startsWith(github.ref, 'refs/tags/artifacts/')`
  condition and using `GH_TOKEN: ${{ github.token }}`. The `workflow_dispatch`
  path (dist/ uploaded as a workflow artifact, no release) is untouched. The
  header comment block and the `permissions: contents: write` comment were
  updated to describe the new attach-only role and note that a manually
  pushed tag needs `gh release create` run first.
- `docs/artifacts.md`: rewrote "Cutting a new release" so a change file
  declaring `artifacts: minor`/`patch`/`major` lands with the kernel/U-Boot
  change (linking to the new `releasing.md` for how change files drive a
  release), merging knope's release PR is now the deliberate human act that
  creates the tag and the GitHub Release with notes from those change files,
  and the Build artifacts workflow only attaches assets to that release. Kept
  the pin-bump PR and three-way verification steps, renumbered them, and
  added an escape-hatch note that a hand-pushed tag needs `gh release create`
  run first since the workflow no longer creates the release itself. Also
  updated the "What's in a release" step 3 and the `workflow_dispatch`
  paragraph, which described the old create-and-publish behavior directly.
