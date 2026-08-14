---
# gosd-9qb0
title: knope config, workflows, and change-file enforcement
status: in-progress
type: feature
priority: normal
created_at: 2026-08-14T06:00:53Z
updated_at: 2026-08-14T06:07:12Z
parent: gosd-vt2l
blocked_by:
    - gosd-dnzo
---

Land the core knope adoption (see epic gosd-vt2l for locked decisions; full TOML/YAML sketches in the approved plan file /Users/jp/.claude/plans/i-d-like-to-automate-fluttering-boot.md). Spike findings (gosd-dnzo) baked in: static PR title/body ($changelog is single-package-only), UNQUOTED change-file frontmatter keys documented, baseline is v0.6.0 (the planned seed change file is obsolete — its content shipped in v0.6.0 and UNRELEASED.md was reset by PR #279).

## Todos

- [x] knope.toml (three packages: gosd/artifacts/npm-gosd; ignore_conventional_commits; document-change / prepare-release / release workflows; STATIC CreatePullRequest title+body — $changelog errors with multiple packages). The spike's proven copy is at /private/tmp/gosd-knope-spike/work/knope.toml
- [x] go.mod module-line comment: `module github.com/jphastings/gosd // v0.6.0`
- [x] build/artifacts/VERSION = 0.10.0 (one bare line)
- [x] Delete docs/releases/UNRELEASED.md (freshly reset stub; superseded by change files — remove the fold-and-reset ritual it describes)
- [x] .github/workflows/prepare-release.yml (push to main; merge-message guard + compgen change-file guard — the latter is ESSENTIAL per spike finding 5; KNOPE_PAT checkout, fetch-depth 0; knope-dev/action SHA-pinned, version 0.23.0)
- [x] .github/workflows/release.yml (pull_request closed, head_ref == knope/release && merged; git config user; knope release; then `git push origin --tags` for the plain Go module tag)
- [x] .github/workflows/change-file-check.yml (require .changeset/*.md in diff OR `no release notes` label; skip for knope/release branch)
- [x] docs/releasing.md (change-file format with the UNQUOTED-key warning, label escape hatch, release-PR semantics, combined-PR ordering discipline, --override-version, 0.x bump rules incl. features→patch, hand-tag escape hatch)
- [x] CLAUDE.md bullet: user-facing PRs need a change file or the label
- [ ] JP (outside the PR): create fine-grained PAT `KNOPE_PAT` (this repo; Contents RW + Pull requests RW) as an Actions secret; create the `no release notes` label; push the one-time bootstrap tag `git tag gosd/v0.6.0 v0.6.0 && git push origin gosd/v0.6.0` (BEFORE the first release-PR merge — else knope creates a spurious gosd/v0.6.0 release, spike finding 4; adjust if the CLI version has moved past 0.6.0 by then)

After merge the release PR will appear once change files exist — HOLD it until gosd-gnnn (artifacts workflow), gosd-96qg (npm docs), and gosd-vxdt (docs cleanup) land.

## Summary of Changes

Adopted knope (v0.23.0, pure GitHub Actions mode) for release automation
across the three release surfaces:

- **`knope.toml`** — three packages (`gosd`, `artifacts`, `npm/gosd`) keyed
  on the exact spike-proven config; `ignore_conventional_commits = true`
  since change files are the only versioning input; static
  `CreatePullRequest` title/body (`$changelog`/`$version` are
  single-package-only variables, so they can't be used with three packages).
- **`go.mod`** — module line gains the `// v0.6.0` comment knope reads as
  the `gosd` package's version source; nothing else touched.
- **`build/artifacts/VERSION`** — new, `0.10.0`, release-automation
  bookkeeping only (the real consumption pin stays
  `internal/artifacts.Version`, bumped tag-first/bump-second as before).
- **`docs/releases/UNRELEASED.md`** — deleted; the fold-and-reset ritual it
  described is superseded by change files. No other doc/workflow/code file
  outside historical `.beans/` records referenced it, so no further fixes
  were needed.
- **Three new workflows**: `prepare-release.yml` (push to main, refreshes
  the `knope/release` PR, guarded against looping on its own merge and
  against running `PrepareRelease` with zero pending change files),
  `release.yml` (fires on the `knope/release` PR merging, tags/publishes
  every pending package, then pushes the plain Go-module tag knope only
  creates locally), and `change-file-check.yml` (requires a
  `.changeset/*.md` in the PR diff or the `no release notes` label). All
  three use the SHA-pinned `actions/checkout@9c091bb...` (`v7.0.0`, this
  repo's dominant pin) and `knope-dev/action@19617851...` (`v2.1.2`,
  resolved via `gh release list`/`gh api`).
- **`.changeset/knope-release-automation.md`** — this PR's own change file
  (`gosd: minor`), doubling as proof the new change-file-check would pass
  on this very PR.
- **`docs/releasing.md`** — new developer doc covering the flow,
  change-file format (with the unquoted-key warning), 0.x bump rules, the
  `no release notes` label, per-surface effects of merging the release PR,
  combined-PR ordering discipline, and the manual tag escape hatch.
- **`CLAUDE.md`** — one new bullet in Workflow, matching the surrounding
  style; no other release-related bullets touched.

Verified: `knope prepare-release --dry-run --verbose` (tokens unset) bumped
`gosd` 0.6.0→0.6.1 from the change file and left `artifacts`/`npm/gosd`
untouched, matching the bean's expectation. `go test ./...`, `go vet ./...`,
`gofmt -l .`, `golangci-lint run ./...`, `GOOS=linux golangci-lint run
./...`, and `actionlint` on the three new workflow files all pass clean.

**Worth flagging for JP:** `docs/releasing.md`'s "manual escape hatch"
section states that hand-pushing an `artifacts/vX.Y.Z` tag now requires
creating the GitHub release first, because the build workflow only uploads
assets to an existing release. That isn't true of `build-artifacts.yml` as
it stands today — it still runs `softprops/action-gh-release` with a
hardcoded name/body, which both creates the release from a bare tag push
and would clobber any knope-authored notes if one already existed. Per this
bean's own brief, that content was specified verbatim, and updating
`build-artifacts.yml` to match is exactly the "artifacts workflow" bean
(gosd-gnnn) this bean's HOLD note already defers to — but until it lands,
the doc describes the target state, not the current one.
