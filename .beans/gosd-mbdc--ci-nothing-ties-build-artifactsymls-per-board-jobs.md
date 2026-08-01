---
# gosd-mbdc
title: 'CI: nothing ties build-artifacts.yml''s per-board jobs to kernelspec.BoardIDs — a new board can pass every test yet ship no artifact'
status: todo
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
