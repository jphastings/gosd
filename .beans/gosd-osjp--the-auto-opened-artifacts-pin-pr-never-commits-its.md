---
# gosd-osjp
title: The auto-opened artifacts pin PR never commits its change file
status: todo
type: bug
created_at: 2026-08-20T05:30:44Z
updated_at: 2026-08-20T05:30:44Z
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

- [ ] Stage the change file in pin-artifacts-version.yml's commit
- [ ] Fail the step if no change file is staged, so a silent regression can't recur
- [ ] Decide whether the already-merged v0.10.2 pin needs a change file adding by hand
