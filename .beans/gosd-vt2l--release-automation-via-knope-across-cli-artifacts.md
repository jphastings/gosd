---
# gosd-vt2l
title: Release automation via knope across CLI, artifacts, and npm
status: in-progress
type: epic
priority: normal
created_at: 2026-08-14T06:00:53Z
updated_at: 2026-08-14T06:10:54Z
---

Adopt knope (knope.tech) for changesets-style release automation across the three release surfaces: gosd CLI (plain vX.Y.Z + gosd/vX.Y.Z tags), board artifacts (artifacts/vX.Y.Z), and npm (npm/gosd/vX.Y.Z). Contributors add .changeset/*.md files per user-facing change; a knope-maintained release PR on branch knope/release accumulates them; merging it tags and creates GitHub releases with real notes; existing tag-triggered pipelines publish unchanged.

Full approved plan: /Users/jp/.claude/plans/i-d-like-to-automate-fluttering-boot.md (copy the relevant locked decisions into each sub-bean).

## Locked decisions (JP, 2026-08-13)

- Tool is **knope v0.23.0**, pure GitHub Actions mode (NO knope-bot App). One fine-grained PAT (`KNOPE_PAT`, this repo only, Contents RW + Pull requests RW) so knope-created tags/PRs trigger other workflows; JP creates it.
- **One combined release PR** across all packages with pending change files. Ordering discipline documented in docs/releasing.md, not enforced in tooling.
- **Auto-open the pin-bump PR** after an artifacts release publishes (own bean; human still does the three-way verification and merges).
- Deliberate amendment of docs/artifacts.md's "no automation pushes tags": the release-PR merge becomes the deliberate human act.
- Three knope packages: `gosd` (root go.mod + `// vX.Y.Z` comment, changelog CHANGELOG.md), `artifacts` (marker file build/artifacts/VERSION, changelog docs/releases/artifacts.md; the consumption pin stays internal/artifacts.Version, tag-first/bump-second), `npm/gosd` (js/packages/gosd/package.json, changelog js/packages/gosd/CHANGELOG.md). publish-npm.yml must stay untouched (its trigger/parsing constrains the npm package name).
- `[changes] ignore_conventional_commits = true` — change files are the only versioning input.
- docs/releases/UNRELEASED.md is retired. (The originally planned seed change file became obsolete mid-epic: v0.6.0 shipped on 2026-08-14 carrying the config-tree call-out, and UNRELEASED.md was freshly reset — adoption starts from a clean v0.6.0 baseline; see spike findings in gosd-dnzo. A one-time bootstrap tag gosd/v0.6.0 -> v0.6.0's commit is required before the first release-PR merge.)
- "No release needed" escape hatch is the `no release notes` PR label (knope has no empty changesets); enforcement via a change-file-check workflow.
