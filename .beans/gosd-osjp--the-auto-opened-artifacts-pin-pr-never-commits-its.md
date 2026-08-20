---
# gosd-osjp
title: The auto-opened artifacts pin PR never commits its change file
status: completed
type: bug
priority: normal
created_at: 2026-08-20T05:30:44Z
updated_at: 2026-08-20T13:08:15Z
---

`build/artifacts/pin-bump.sh` writes `.changeset/artifacts-pin-vX-Y-Z.md`
precisely so a `Version` bump ships in a CLI release — its own comment says
"Without a change file, knope cuts no CLI release and the bump reaches nobody
who installs gosd rather than building it from source."

`.github/workflows/pin-artifacts-version.yml` then stages only the Go file:

```
git add internal/artifacts/artifacts.go
git commit -m "Bump internal/artifacts.Version to $version" ...
```

`git commit` without `-a` commits only what is staged, so the change file is
left untracked in the runner and thrown away with it. Confirmed on the last
real run: PR #306 / commit 072f66d (the v0.10.2 bump) touched
`internal/artifacts/artifacts.go` and nothing else.

Consequence: every automated pin bump sits on `main` contributing nothing to
any release, which is the exact failure the change-file step was added to
prevent — and it looks like it worked, because the script prints that it
wrote the file.

Found while implementing gosd-1jjh (which teaches the same script to write
`ManifestSHA256` alongside `Version`), deliberately not fixed there: it is
gosd-odx3's mechanism and deserves its own review.

## Fix

Stage the change file too — `git add internal/artifacts/artifacts.go .changeset`
in the "Push the branch and open the PR" step (the checkout is fresh from
`main`, so the only untracked file under `.changeset/` is the one pin-bump.sh
just wrote). Worth asserting it landed, rather than trusting it: fail the step
if `git diff --cached --name-only` doesn't list a `.changeset/` path.

## Todos

- [x] Stage the change file in pin-artifacts-version.yml's commit
- [x] Fail the step if no change file is staged, so a silent regression can't recur
- [x] Decide whether the already-merged v0.10.2 pin needs a change file adding by hand: yes — added `.changeset/artifacts-pin-v0-10-2.md` by hand, since no CLI release has been cut since commit 072f66d (the last `gosd/v*` tag is v0.6.5), the bump is still sitting unreleased and fully recoverable.

## Summary of Changes

`.github/workflows/pin-artifacts-version.yml`'s "Push the branch and open the PR" step now runs `git add internal/artifacts/artifacts.go .changeset` (was: the Go file only) and fails the step with an actionable `::error::` if `git diff --cached --name-only` shows no `.changeset/` path staged, so a future regression (e.g. pin-bump.sh stops writing a change file) is caught at the workflow rather than silently landing on main again.

Also added `.changeset/artifacts-pin-v0-10-2.md` by hand for the already-merged commit 072f66d (PR #306), reconstructed from `docs/releases/artifacts.md`'s 0.10.1/0.10.2 sections the same way pin-bump.sh would have generated it. This is fully recoverable rather than a lost cause because no `gosd/v*` CLI release has been cut since (the last tag is v0.6.5) — the bump was still sitting in the unreleased backlog, just missing its change file, not already shipped without one.
