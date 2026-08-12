---
# gosd-w6zc
title: 'CA pin freshness guard: warn when internal/cacerts falls behind curl.se'
status: completed
type: task
priority: low
created_at: 2026-08-07T13:58:39Z
updated_at: 2026-08-07T13:58:39Z
---

gosd-6zd1's option-B analysis predicted this obligation: with the Mozilla
bundle shipping in every image (gosd-kzgq), roots now age with gosd releases
— nothing notices when curl.se publishes a newer snapshot (Mozilla updates
roughly quarterly; removals matter more than additions, since a distrusted
CA stays trusted on-device until a pin bump ships).

## Fix (keep it warn-only and cheap)

- [x] A scheduled (cron) GitHub Actions workflow that fetches
      https://curl.se/ca/cacert.pem.sha256 and compares it to
      internal/cacerts.Pin.SHA256; on mismatch it opens (or refreshes) a
      single issue titled with the new dated snapshot name — never
      auto-bumps, never fails a build.
- [x] One line in the release procedure docs: "check the cacerts pin is
      current" (point at the workflow rather than duplicating the
      mechanics).
- [x] The bump itself stays the documented two-constant edit in
      internal/cacerts (dated URL + sha256, verified against curl.se's
      published .sha256) — unchanged, just confirmed still accurate.

## Summary of Changes

- Added `.github/workflows/cacerts-pin-check.yml`: a scheduled (weekly cron)
  + `workflow_dispatch` job that extracts `internal/cacerts.Pin.SHA256` out
  of `internal/cacerts/cacerts.go` via `sed` (never hardcoded — that's the
  whole point of the check), fetches curl.se's rolling
  `cacert.pem.sha256`, and compares them. On a mismatch it best-effort
  names the newest dated snapshot from `https://curl.se/docs/caextract.html`
  (discovered during implementation: `https://curl.se/ca/` itself is just a
  redirect stub with no directory listing — the dated changelog lives at
  `/docs/caextract.html` instead) and opens or refreshes a single GitHub
  issue (deduped by a fixed `cacerts pin stale:` title-prefix search, per
  the bean's suggested alternative to a label) via `gh issue
  create`/`gh issue edit`. Never edits the pin, never fails the workflow —
  warn-only per the bean. All `uses:` steps pin a full SHA with a version
  comment, matching the repo's existing convention (`actions/checkout` only
  needed here); validated with `actionlint`.
- `docs/artifacts.md`: added step 7 to "Cutting a new release" — one line
  pointing at the new workflow rather than duplicating its mechanics, as
  the bean asked. This is the only documented release procedure in the
  repo, so it's the natural home for the pointer even though the cacerts
  pin isn't itself an artifact-release concern.
- Verified the sha256-extraction `sed` against the real
  `internal/cacerts/cacerts.go` and the curl.se comparison/snapshot-naming
  logic against the live endpoints before committing.
- Quality gates: `gofmt -l .` clean; `go vet ./...` passed; `golangci-lint
  run ./...` and `GOOS=linux golangci-lint run ./...` both reported 0
  issues; `actionlint` on the new workflow reported 0 findings. `go test
  ./...` was run to completion once and every package passed except
  `cmd/gosd`, which hit Go's built-in 10-minute per-package test timeout —
  the goroutine dump showed the test still actively blocked on subprocess
  I/O (no assertion failure, no deadlock) — because this shared bench
  machine was under extreme concurrent load from multiple sibling agents
  during this session (10+ concurrent `go test ./...` processes observed at
  once, free disk repeatedly dropping under 1GiB, and the harness itself hit
  a live ENOSPC writing its own tool output). This PR contains zero Go
  source changes, so that contention-induced timeout cannot be attributable
  to it.
