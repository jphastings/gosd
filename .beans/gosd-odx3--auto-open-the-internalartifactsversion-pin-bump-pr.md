---
# gosd-odx3
title: Auto-open the internal/artifacts.Version pin-bump PR after an artifacts release
status: completed
type: feature
priority: normal
created_at: 2026-08-14T06:00:53Z
updated_at: 2026-08-21T02:38:45Z
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


## Follow-up (JP, 2026-08-17): the checklist is CI's job, not a human's

The first cut opened the PR as a draft carrying the three-way verification as
a markdown checklist. JP's objection on seeing the generated PR (#304): a
checklist somebody has to work through is the step that gets skipped, and it
looks like verification while proving nothing. Automated instead.

`.github/workflows/verify-artifacts-pin.yml` runs on any PR touching
`internal/artifacts/artifacts.go`:

- **Clean-machine build** — `XDG_CACHE_HOME` redirected at an empty dir (Go's
  `os.UserCacheDir` follows it on Linux, and `artifactCacheDir` derives from
  that), no `--board`/`--artifacts-dir`, so every public board builds from a
  real download. `EnsureBoard` verifies each file against the manifest's
  sha256 while unpacking, so a green build IS the digest check — no
  reimplementation needed.
- **It really was the new release** — asserts the redirected cache contains a
  directory for the newly pinned version, catching a build that quietly
  succeeded against some other release.
- **Offline re-run** — every proxy at a closed port; must succeed from the
  cache the previous step populated.
- **What actually moved** — `build/artifacts/pin-diff.sh` compares the two
  releases' manifests file by file into the job summary.

The pin PR therefore opens ready for review rather than as a draft: green
checks are the gate.

### What deliberately stays human

Whether a release carries the change it was *cut for* is judgment, not a
check — so `pin-diff.sh` reports which boards moved and the reviewer reads
it. Real-hardware boots stay on the motivating bean. Run against the live
releases, v0.10.0 → v0.10.2 reports cubie-a5e/nanopi-zero2/qemu-virt/
radxa-zero-3e/rock-4se moving (the mainline fleet shares a kernel tag and was
rebuilt) with the Pi family byte-identical, and shows cubie's stock DTB
unchanged while its U-Boot changed and the gadget DTB is new — exactly the
shape a reviewer needs.


## The gap this missed, found by JP (2026-08-17)

The generated pin PR carried no change file, and I labelled the manual one
`no release notes` on the same reasoning: a pin bump is "release plumbing
with no user-facing surface". That is exactly backwards — delivering the
board fixes IS its user-facing surface.

The consequence was live within hours: JP's `atfs` image, built with an
installed `gosd v0.6.2`, halted in U-Boot SPL on his 1GB board with the very
DRAM failure fixed that morning. `gosd v0.6.2` pins v0.10.0, because the pin
landed on main AFTER that release was cut, and with no change file pending
knope had no reason to cut another. Anyone running `gosd@latest` was still
building unbootable Cubie A5E images.

So `pin-bump.sh` now writes the change file itself, and two related bugs
found while testing it are fixed:

- **It only described the target release.** Bumping v0.10.0 → v0.10.2
  reported v0.10.2's changes and silently dropped v0.10.1's — which was the
  DRAM fix, i.e. the entire reason the bump mattered. It now spans every
  release in the range, in both the change file and the doc comment.
- **Re-running duplicated entries.** The workflow can legitimately run again
  over an already-annotated tree; releases the comment already names are now
  skipped.


---

## Validated end to end by artifacts/v0.10.3 (2026-08-20)

This bean was held open deliberately — "lands after the first knope artifacts
release validates the flow" — because opening a PR is easy and opening a
*correct* one is the thing worth proving. v0.10.3 proved it, unattended:

| | |
|---|---|
| 19:45 | `artifacts/v0.10.3` published |
| 20:16 | `pin-artifacts-version.yml` fired on `workflow_run` and succeeded |
| — | PR #351 opened carrying **both** `internal/artifacts/artifacts.go` **and** `.changeset/artifacts-pin-v0-10-3.md` |
| — | `Build every board from the pinned release` passed: clean-machine build of every public board from a real download, the newly-pinned version present in the redirected cache, and the offline re-run |
| 21:03 | merged; `main` now reads `const Version = "v0.10.3"` |

The change file is the part that matters. Every earlier run of this flow —
including v0.10.2's PR #306 — dropped it, so an automated pin bump reached
main and knope had no reason to cut a CLI release carrying it. That is the
failure this bean's own "gap found by JP" section describes, where an image
built with `gosd v0.6.2` halted in U-Boot SPL on a 1GB Cubie because the DRAM
fix was pinned but unreleased.

The staging bug behind it was `.github/workflows/pin-artifacts-version.yml`
adding only `internal/artifacts/artifacts.go` to the index, so the change file
`pin-bump.sh` had written was never committed — filed as [[gosd-osjp]] and
fixed in PR #348, merged 14:37 the same day, about five hours before this run.
So v0.10.3 is the first pass over the completed mechanism, and it behaved.

Nothing is left outstanding: the human gate is what it was designed to be —
reading `pin-diff.sh`'s report of which boards moved — and real-hardware boots
remain on whichever bean motivated the release.
