---
# gosd-mbdc
title: 'CI: nothing ties build-artifacts.yml''s per-board jobs to kernelspec.BoardIDs — a new board can pass every test yet ship no artifact'
status: completed
type: task
priority: normal
created_at: 2026-07-31T07:53:52Z
updated_at: 2026-07-31T07:53:52Z
---

Found by review sweep `gosd-fuxs` (kernel/CI infra area), verified.

kernelspec_test.go pins BoardIDs() against Go-level lists only. Nothing
cross-checks build-artifacts.yml's job list, the `needs:` array on
package-and-release, or its download-artifact steps against that set.
`fail_on_unmatched_files: true` catches a listed-but-missing release file,
but forgetting the job/needs/download step entirely produces no failure
at all. CLAUDE.md documents the workflow wiring as a purely manual step.

**Failure scenario:** a new board lands in kernelspec + boards, all Go
tests green, workflow never updated → tag-triggered release succeeds
silently minus that board → users get a download-404 far downstream.

**Fix:** a Go test that parses build-artifacts.yml (it's in-repo YAML) and
asserts the per-board job names / needs list / download steps form a
superset of kernelspec.BoardIDs(), failing in the same PR that adds the
board.

## Todos

- [x] Go test in internal/kernelspec that parses .github/workflows/build-artifacts.yml
- [x] Checks per board: a `<board>-kernel` job exists, it's in package-and-release's needs, and package-and-release downloads the artifact it uploads
- [x] Checks the reverse: every `<X>-kernel` job names a board kernelspec.BoardIDs() knows about (stale-job guard)
- [x] Actionable failure messages naming the missing job/needs entry/download step
- [x] Quality gates + PR

## Summary of Changes

- Added `internal/kernelspec/workflow_test.go`, a Go test (no new workflow
  machinery) that reads `.github/workflows/build-artifacts.yml` as plain
  YAML (relative path from the package dir, same pattern
  `TestPiRequiredYIsDerivedFromFragment` already uses for
  `build/boards/*/kernel.fragment`) via the already-direct `gopkg.in/yaml.v3`
  dependency.
- The test discovers every per-board kernel job by its existing
  `<board>-kernel` naming convention (rather than hard-coding the board
  list, which would just recreate the drift this test exists to catch), then
  for every board in `kernelspec.BoardIDs()` asserts: (1) a matching
  `<board>-kernel` job exists, (2) `package-and-release`'s `needs:` list
  includes it, and (3) `package-and-release` has a `download-artifact` step
  for the artifact name that job's `upload-artifact` step uploads. It also
  checks the reverse direction — every `<X>-kernel` job's `X` must be a board
  kernelspec knows about — to catch a stale job left behind by a removed
  board.
- Failure messages name the missing job/needs-entry/download-step and the
  board, e.g. `board "X" has a kernelspec entry but no "X-kernel" job in
  .github/workflows/build-artifacts.yml; add one ... and wire it into
  package-and-release's needs/download-artifact steps` — verified by
  temporarily mutating a scratch copy of the workflow (renamed job, dropped
  needs entry) and confirming the test fails with the expected actionable
  message, then restoring the file byte-for-byte (confirmed via diff)
  before committing.
- No production code changed; `.github/workflows/build-artifacts.yml` itself
  is untouched (PR #209, unmerged, touches only `ci.yml` — no overlap).
- Gates green: `go test ./...`, `go vet ./...`, `gofmt -l .`,
  `golangci-lint run ./...`, `GOOS=linux golangci-lint run ./...`.
