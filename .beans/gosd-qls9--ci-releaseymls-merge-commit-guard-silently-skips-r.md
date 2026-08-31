---
# gosd-qls9
title: 'CI: release.yml''s merge-commit guard silently skips releases'
status: completed
type: bug
priority: normal
created_at: 2026-08-31T11:56:02Z
updated_at: 2026-08-31T12:02:59Z
---

`release.yml`'s trigger — `contains(github.event.head_commit.message, 'chore:
prepare release')` on `push: branches: [main]` — silently skipped after
merging the knope release PR #381: GitHub's default "Create a merge commit"
button writes the merge commit subject as "Merge pull request #381 from
jphastings/knope/release" and does NOT fold the PR's title into the
message, so the guard never matched. No error, no failed check — just a
skipped job. Recovered manually via an empty `--allow-empty` commit
containing the phrase, which retriggered `release.yml` against main's
already-prepared state (v0.8.3 / artifacts 0.10.4 shipped 2026-08-31).

## Locked decisions

- **Do NOT switch release.yml to a `pull_request`-triggered workflow.**
  Verified via `gh api repos/jphastings/gosd/environments/knope-release`:
  the `knope-release` environment has a custom deployment branch policy
  restricted to `main` only. A `pull_request`-triggered run's `github.ref`
  is `refs/pull/<N>/merge`, not `refs/heads/main`, so it would be refused
  the environment's secrets outright — turning a silent skip into a hard
  failure on every single release. This was seriously considered and
  rejected for this reason; do not relitigate without first re-checking
  that environment's branch policy.
- **Fix instead by widening both workflows' existing `contains()` guards**
  to also match the branch name `knope/release`, which IS reliably present
  in GitHub's default merge-commit subject line regardless of whether the
  PR title made it into the message body. Keep the `push: branches: [main]`
  trigger unchanged on both files (branch-policy compatibility depends on
  it).
  - `.github/workflows/release.yml`: `if:` becomes
    `github.event_name == 'workflow_dispatch' || contains(github.event.head_commit.message, 'chore: prepare release') || contains(github.event.head_commit.message, 'knope/release')`.
    Added `workflow_dispatch:` alongside the existing `push:` trigger as a
    clean manual recovery lever (dispatched against `main`, so
    `github.ref == refs/heads/main` — still branch-policy-compatible).
  - `.github/workflows/prepare-release.yml`: negate the same widened
    condition: `if: "!(contains(github.event.head_commit.message, 'chore: prepare release') || contains(github.event.head_commit.message, 'knope/release'))"`.
    Its own internal "Check for pending change files" step is the real
    safety net either way (safe no-op when `.changeset/` is empty), so
    this widening is belt-and-suspenders, not load-bearing.
  - Both files' explanatory comments updated to describe why the check
    covers both the PR-title-in-message case (squash-merge-style) and the
    default-merge-commit-subject case (branch-name match), and to note
    `workflow_dispatch` as the recovery path (not an empty commit hack).
- **docs/releasing.md**: replaced the "Don't retitle the release PR"
  warning box with an accurate account of both matched forms, plus a
  concrete "how to recognize + recover from a skipped release" procedure
  (compare the changelog/go.mod version on main against existing tags;
  dispatch `release.yml` manually rather than an empty commit).
- No Go files touched — CI config + docs only. No changeset: internal CI
  fix, not a user-facing package release note (`no release notes` label
  applied to the PR instead, per docs/releasing.md's own rule).

## Todos

[x] .github/workflows/release.yml: widen the `if:`, add `workflow_dispatch:`,
    update comments
[x] .github/workflows/prepare-release.yml: widen the negated `if:`, update
    comments
[x] docs/releasing.md: replace the "Don't retitle" warning box with the
    accurate account + recovery procedure
[x] Validate YAML syntax; confirm CI's own workflow-lint (if any) is green
[x] Apply `no release notes` label to the PR

## Summary of Changes

Fixed the actual bug from PR #381's silently-skipped release, and hardened
against recurrence:

- `internal/repocheck`'s `TestDocPathsAreLinkedNotNamed` caught a bare
  `CHANGELOG.md` path reference introduced by the new docs/releasing.md
  prose — fixed by hyperlinking a descriptive phrase per this repo's own
  markdown convention, exactly the kind of thing that check exists to
  catch.
- Verified with `actionlint` (both edited workflow files clean) and the
  full local gate (`go build`, `go test`, `go vet`, `gofmt`,
  `golangci-lint`) — no Go files were touched, so these are unaffected/
  clean by construction, run anyway per the project's standing quality-gate
  rule.
- Deliberately did NOT switch `release.yml` to a `pull_request` trigger —
  investigated and rejected after confirming via the GitHub API that the
  `knope-release` environment's branch policy would refuse it. Recorded as
  a locked decision above so it isn't relitigated without re-checking that
  policy first.
