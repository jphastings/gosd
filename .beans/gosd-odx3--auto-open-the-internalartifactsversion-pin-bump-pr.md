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

- [x] Final job on build-artifacts.yml after asset upload (tag runs only): checkout main; sed `const Version = "v…"` in internal/artifacts/artifacts.go to `${GITHUB_REF_NAME#artifacts/}`; splice the newest docs/releases/artifacts.md section into the doc-comment mini-changelog; `gh pr create` via an app installation token (NOT the default GITHUB_TOKEN — a GITHUB_TOKEN-created PR gets no CI). Since this job runs on a tag push, minting from the knope-release environment needs its deployment policy extended to allow `artifacts/v*` tags — decide then whether to extend it or add a sibling environment
- [x] PR body names the three-way verification (clean-machine build, offline re-run, dtb spot-check) as the human gate; curated-prose polish happens in review
- [x] docs/artifacts.md: pin-bump steps now start from the auto-opened PR


## Departure from the sketched approach: a workflow_run workflow, not a job inside build-artifacts.yml

The todo above said "final job on build-artifacts.yml". It is instead a
separate workflow triggered by `workflow_run` on that one completing, for two
reasons that only became visible while doing it:

1. **A job inside build-artifacts.yml can never serve a release that
   predates it.** Tag-triggered runs execute the workflow file *as of the
   tag*, so adding the job today does nothing for `artifacts/v0.10.2` (already
   built), and re-running that run re-reads the same old file. `workflow_run`
   events run the file from the default branch, so the logic is always the
   current one and can be dispatched against any already-published release.
   That is what lets this generate v0.10.2's PR without cutting a pointless
   release.
2. **It removes the environment-policy decision this bean flagged.** A
   `workflow_run` job runs in the default branch's context, so the
   `knope-release` environment needs no widening to cover `artifacts/v*` tag
   refs, and no sibling environment is needed. The app-token minting is
   copied verbatim from release.yml.

Everything else follows the sketch: app installation token (not
`GITHUB_TOKEN`, which yields a PR with no CI), the constant rewritten from
the tag, the release's own notes spliced into the doc comment, and the
three-way verification named in the body as the human gate.

## Summary of Changes

- `build/artifacts/pin-bump.sh` — rewrites the constant and splices the
  release notes into its doc comment. Standalone and side-effect-free (no
  commit, push, or PR) so it can be run against a real checkout, the same
  reasoning as its sibling `package.sh`. Exit 3 means "already pinned", which
  is how the workflow avoids opening duplicate PRs.
- `build/artifacts/pin-bump-pr-body.md` — the PR body, as a template rather
  than heredoc'd inside YAML, so the prose can be edited and reviewed.
- `.github/workflows/pin-artifacts-version.yml` — mints the app token, checks
  the release actually has assets before pinning anything, runs the script,
  and opens the PR **as a draft**: the pin is only sound once verified, and
  draft status is what says so.
- `docs/artifacts.md` — step 4 now starts from the auto-opened PR, with the
  manual fallback for releases that predate the automation.

### Verified locally

The script was exercised against the real repo before any CI existed:
accepts `v0.10.2`, `0.10.2` and `artifacts/v0.10.2` identically; exits 2 on a
malformed version, 1 when the changelog has no such section, 3 when already
pinned, 0 on success; and produces a compact one-entry doc comment
(`v0.10.2: The Cubie A5E kernel build now produces a USB-gadget variant
device tree; Cubie A5E U-Boot no longer scans USB on every boot.`) rather
than dumping whole changelog bodies, which the first draft did.
