---
# gosd-7llw
title: gosd build never removes its temp directory, filling $TMPDIR
status: completed
type: bug
priority: high
created_at: 2026-08-12T09:24:11Z
updated_at: 2026-08-12T09:32:36Z
---

`gosd build` creates its working directory with
`os.MkdirTemp("", "gosd-build-")` (`cmd/gosd/build.go`) and never removes it —
there is no `defer os.RemoveAll`, on the error paths or the successful one.
`grep -n tempDir cmd/gosd/build.go` returns exactly two lines: the creation and
the pass into `compileForBoards`. The directory is abandoned on every
invocation, and has been since the first commit (664343a).

Each leaked directory holds the cross-compiled app binary plus `gosd-init` per
architecture: ~17MB for a single board, ~70MB for a default all-boards build.
On JP's machine these accumulated to tens of GB in `$TMPDIR`
(`/var/folders/…/T/` on macOS, which is not reliably reaped) and became a
disk-space problem.

This is the only temp dir in the repo without cleanup. Every sibling has one:
`gosd run` (`cmd/gosd/run.go`, conditional on `--keep`),
`internal/qemurun`, `internal/extbuild`, `internal/kernelbuild`,
`internal/artifacts`, `internal/fetch`.

**The dominant amplifier is the test suite, not manual builds.** 62 test cases
across `cmd/gosd/*_integration_test.go` invoke the `build` command, and none
redirect `TMPDIR`, so a single `go test ./cmd/gosd/...` leaks ~62 directories —
one to a few GB per run, multiplied by every agent session and every worktree.
It affects anyone who builds or tests gosd, not just this machine.

## Fix

Add `defer func() { _ = os.RemoveAll(tempDir) }()` immediately after the
`MkdirTemp` error check, in the same shape as `qemurun`/`extbuild`.
Function-scoped is correct: everything that reads out of `tempDir` (the
`binaries` map, consumed by `pipeline.Assemble`) lives inside `runBuild`, and
`main`'s `os.Exit(1)` happens after cobra has returned the error up, so the
defer still fires on failed builds.

Regression test in `cmd/gosd/build_integration_test.go`: point `TMPDIR` at
`t.TempDir()`, run a single-board build, assert no `gosd-build-*` remains.

## Scope decisions (JP, 2026-08-12)

- **No self-healing sweep of pre-existing leaked dirs.** Reclaiming space
  already lost is a manual `rm -rf "$TMPDIR"/gosd-build-*`.
- **Interrupted builds still leak.** `gosd build` installs no signal handler,
  so Ctrl-C kills the process before any defer runs. Far smaller in volume than
  the test-suite leak; recorded here so it isn't mistaken for a regression.

## Todos

- [x] `defer os.RemoveAll(tempDir)` in `runBuild`
- [x] Regression test asserting no `gosd-build-*` survives a build
- [x] Quality gates green (test, vet, gofmt, golangci-lint darwin + linux)

## Measured on this machine, 2026-08-12

The amplifier estimate turned out to be conservative, and it was caught in the
act: partway through this fix, a parallel session's `go test ./cmd/gosd/...`
left **71 directories totalling 1.3GB**, all created in one burst between
08:12 and 08:34. Their contents are the test suite's board mix exactly —
`gosd-init-arm64` in 70 of them, `app-pi-zero-2w` in 51, plus `app-qemu-virt`,
`app-pi-3b`, `app-radxa-zero-3e`, `app-nanopi-zero2`, `app-pi-zero-w`,
`gosd-init-arm-6` and `gosd-tsfunnel-arm64`. So ~1.3GB per full test run, per
worktree, per session — roughly twenty runs to reach the "tens of GB" that
prompted this bean.

The same run repeated after the fix added zero.

## Summary of Changes

- `cmd/gosd/build.go`: `runBuild` now does
  `defer func() { _ = os.RemoveAll(tempDir) }()` immediately after creating its
  working directory, matching `internal/qemurun` and `internal/extbuild`. This
  is the whole behavioural fix; every other temp dir in the repo already had it.
- `cmd/gosd/build_integration_test.go`: new
  `TestBuildRemovesItsTempDirectory` points `TMPDIR` at `t.TempDir()` (which
  `os.TempDir` re-reads on every call), runs a single-board build, and asserts
  no `gosd-build-*` survives. Verified to fail against the unfixed `build.go`,
  naming the leaked path.
