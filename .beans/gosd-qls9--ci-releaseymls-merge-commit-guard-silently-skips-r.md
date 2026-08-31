---
# gosd-qls9
title: 'CI: release.yml''s merge-commit guard silently skips releases'
status: in-progress
type: bug
created_at: 2026-08-31T11:56:02Z
updated_at: 2026-08-31T11:56:02Z
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
    Add `workflow_dispatch:` alongside the existing `push:` trigger as a
    clean manual recovery lever (dispatched against `main`, so
    `github.ref == refs/heads/main` — still branch-policy-compatible).
  - `.github/workflows/prepare-release.yml`: negate the same widened
    condition: `if: "!(contains(github.event.head_commit.message, 'chore: prepare release') || contains(github.event.head_commit.message, 'knope/release'))"`.
    Its own internal "Check for pending change files" step is the real
    safety net either way (safe no-op when `.changeset/` is empty), so
    this widening is belt-and-suspenders, not load-bearing — but keep it
    accurate rather than leave a guard that no longer matches its own
    "exact complement of release.yml's guard" comment.
  - Update both files' explanatory comments to describe why the check
    covers both the PR-title-in-message case (squash-merge-style) and the
    default-merge-commit-subject case (branch-name match), and to note
    `workflow_dispatch` as the recovery path (not an empty commit hack).
- **docs/releasing.md**: replace the existing "Don't retitle the release
  PR" warning box with an accurate account: the routing depends on the
  merge commit's message containing either the PR's title OR the source
  branch name `knope/release` (GitHub's default merge-commit subject
  always includes the latter); retitling the PR *and* somehow avoiding
  both matches would still break it, but the routing is no longer solely
  dependent on the title surviving into the message. Explain how to
  recognize a skipped release (CHANGELOG.md/go.mod on main show the bumped
  version but no matching `vX.Y.Z`/`gosd/vX.Y.Z` tag exists) and the fix
  (`gh workflow run release.yml` — or the Actions UI — dispatched against
  `main`, now that `workflow_dispatch` exists; no more empty-commit hack).
- No Go files touched — this is CI config + docs only. No changeset
  needed for user-facing release notes (apply `no release notes` label:
  this is an internal CI fix, not a package the `gosd`/`artifacts`/
  `npm/gosd` change-file schema covers), per docs/releasing.md's own
  "not every PR has a release note" rule.

## Todos

[ ] .github/workflows/release.yml: widen the `if:`, add `workflow_dispatch:`,
    update comments
[ ] .github/workflows/prepare-release.yml: widen the negated `if:`, update
    comments
[ ] docs/releasing.md: replace the "Don't retitle" warning box with the
    accurate account + recovery procedure
[ ] Validate YAML syntax; confirm CI's own workflow-lint (if any) is green
[ ] Apply `no release notes` label to the PR (or confirm CI's change-file
    check accepts it)

## Summary of Changes
