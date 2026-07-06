---
# gosd-5vju
title: gosd build -o fails when the output directory doesn't exist
status: completed
type: bug
priority: normal
created_at: 2026-07-06T12:09:09Z
updated_at: 2026-07-06T12:09:45Z
---

Repro: `go run ./cmd/gosd build ./examples/hello --catalog --publish-base-url=http://localhost:8000/ -o /tmp/gosd-imager` fails with "creating image file ...: no such file or directory" when /tmp/gosd-imager does not already exist (multi-board build, so -o names a directory). Expected: gosd build creates the output directory (MkdirAll, 0755) before building, per the principle of least surprise, instead of crashing partway through cross-compiling/building. Same applies to the single-board case where -o names a file whose parent directory doesn't exist yet — its parent should be created too. An -o that already exists as a plain file in multi-board mode should get an actionable error instead of a raw filesystem error.


## Todos
- [x] Multi-board mode: MkdirAll the -o directory (0755) before building
- [x] -o existing as a plain file in multi-board mode: actionable error naming the path
- [x] Single-board mode: MkdirAll -o's parent directory before building
- [x] Verify the catalog writer (writes next to the images) benefits from the same guarantee
- [x] Behavioral tests: multi-board build into a nonexistent nested directory; single-board -o with nonexistent parent; multi-board -o pointing at an existing file fails actionably
- [x] Unit tests for the path-resolution/dir-creation function
- [x] Quality gates: go test ./..., go vet ./..., gofmt -l ., golangci-lint run ./... (darwin + GOOS=linux)

## Summary of Changes
Added `ensureOutputDir(output string, multiBoard bool) error` in cmd/gosd/build.go, called from runBuild right after resolveOutputs and before any cross-compiling/building starts. In multi-board mode it MkdirAll's the -o directory itself (defaulting to "." when -o is empty); in single-board mode it MkdirAll's the parent of the -o file path (a no-op when -o is empty, since "." always exists). If MkdirAll fails in multi-board mode because -o already exists as a plain file, it's reported as "-o must be a directory when building multiple boards; X is a file" instead of the underlying MkdirAll error. Any other MkdirAll failure (e.g. permissions) is wrapped into an actionable message naming the path and suggesting a fix, per CLAUDE.md's error-message convention.

Since the output directory now exists before pipeline.Assemble runs for any board, writeCatalog (which writes os_list.json next to the first image) automatically benefits — no separate change needed there.

Single-board mode's existing behavior when -o itself already exists as a directory is unchanged: image.Write still fails when it tries to open that path as a file (diskfs.Create surfaces the underlying "is a directory" OS error) — only the previously-crashing missing-parent-directory case was fixed.

Added unit tests (cmd/gosd/build_test.go) for ensureOutputDir covering: missing multi-board directory creation, missing single-board parent creation, multi-board rejection of an existing file with the actionable message, and the empty-output no-op. Added integration tests (cmd/gosd/build_integration_test.go) exercising the full `gosd build` command: multi-board build into a nested nonexistent directory, single-board build with a nonexistent parent directory, and multi-board build with -o pointing at an existing file (expects the actionable error).
