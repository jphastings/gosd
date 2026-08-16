---
# gosd-vt2l
title: Release automation via knope across CLI, artifacts, and npm
status: in-progress
type: epic
priority: normal
created_at: 2026-08-14T06:00:53Z
updated_at: 2026-08-14T06:51:20Z
---

Adopt knope (knope.tech) for changesets-style release automation across the three release surfaces: gosd CLI (plain vX.Y.Z + gosd/vX.Y.Z tags), board artifacts (artifacts/vX.Y.Z), and npm (npm/gosd/vX.Y.Z). Contributors add .changeset/*.md files per user-facing change; a knope-maintained release PR on branch knope/release accumulates them; merging it tags and creates GitHub releases with real notes; existing tag-triggered pipelines publish unchanged.

Full approved plan: /Users/jp/.claude/plans/i-d-like-to-automate-fluttering-boot.md (copy the relevant locked decisions into each sub-bean).

## Locked decisions (JP, 2026-08-13)

- Tool is **knope v0.23.0**, pure GitHub Actions mode (NO knope-bot App). Credentials: a self-owned GitHub App (Contents RW + Pull requests RW, installed on this repo only) whose private key lives in the `knope-release` environment (deployment branches restricted to main); workflows mint 1-hour auto-revoked installation tokens via actions/create-github-app-token, so knope-created PRs/tags trigger other workflows without any long-lived user credential (amended from a fine-grained PAT 2026-08-14 over exfiltration concerns).
- **One combined release PR** across all packages with pending change files. Ordering discipline documented in docs/releasing.md, not enforced in tooling.
- **Auto-open the pin-bump PR** after an artifacts release publishes (own bean; human still does the three-way verification and merges).
- Deliberate amendment of docs/artifacts.md's "no automation pushes tags": the release-PR merge becomes the deliberate human act.
- Three knope packages: `gosd` (root go.mod + `// vX.Y.Z` comment, changelog CHANGELOG.md), `artifacts` (marker file build/artifacts/VERSION, changelog docs/releases/artifacts.md; the consumption pin stays internal/artifacts.Version, tag-first/bump-second), `npm/gosd` (js/packages/gosd/package.json, changelog js/packages/gosd/CHANGELOG.md). publish-npm.yml must stay untouched (its trigger/parsing constrains the npm package name).
- `[changes] ignore_conventional_commits = true` — change files are the only versioning input.
- docs/releases/UNRELEASED.md is retired. (The originally planned seed change file became obsolete mid-epic: v0.6.0 shipped on 2026-08-14 carrying the config-tree call-out, and UNRELEASED.md was freshly reset — adoption starts from a clean v0.6.0 baseline; see spike findings in gosd-dnzo. A one-time bootstrap tag gosd/v0.6.0 -> v0.6.0's commit is required before the first release-PR merge.)
- "No release needed" escape hatch is the `no release notes` PR label (knope has no empty changesets); enforcement via a change-file-check workflow.

## Progress (2026-08-16)

Shipped and proven end to end: knope config + workflows (#281, app-token auth), attach-only artifacts workflow (#280), docs (#282, #283), the package.json newline fix (#286). Real releases through the pipeline: gosd 0.6.1 (release PR #284; `gosd/v0.6.1` + plain `v0.6.1` module tag) and npm/gosd 0.3.1 (release PR #287; its tag fired publish-npm.yml — the app-token-triggers-workflows mechanism artifacts will rely on). `release.yml` has correctly skipped on every ordinary merge since. Only gosd-odx3 (pin-bump auto-PR) remains, held for the first knope artifacts release.

