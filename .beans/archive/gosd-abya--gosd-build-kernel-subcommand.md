---
# gosd-abya
title: gosd build-kernel subcommand
status: completed
type: feature
priority: normal
created_at: 2026-07-11T07:41:32Z
updated_at: 2026-07-11T11:53:45Z
parent: gosd-47rm
blocked_by:
    - gosd-x488
---

Part of [[gosd-47rm]]. The user-facing subcommand wiring KernelSpec
([[gosd-di6v]]) + container runtime ([[gosd-fe9w]]) + builder
([[gosd-x488]]).

## Locked decisions

- `cmd/gosd/buildkernel.go`, `newBuildKernelCmd()`, registered in
  `newRootCmd()` — same shape as `newBuildCmd`/`newRunCmd`. The board registry
  from `build.go`'s `init()` is already available.
- **Flags:**
  - `--board` repeatable; default = all public boards (qemu-virt only when
    explicitly named, mirroring `gosd build`)
  - `-o/--output <dir>` (default `./gosd-artifacts/`): flat drop-in
    `--artifacts-dir` layout
  - `--config <path>` (default: `gosd-kernel.toml` in the working directory if
    present): developer overlay; the flag parses/threads it even if the full
    schema lands in the follow-up config bean — start with fragment/patches
    overlay support
  - `--builder docker|podman` (default auto-detect)
  - `--staging <dir>` (CI use): additionally emit the `staging/<board>/`
    layout `package.sh` consumes, incl. `source.json`
- Help text says up front this command needs Docker or Podman and that most
  developers don't need it (stock artifacts are the default path).
- On success, print the follow-up invocation:
  `gosd build --artifacts-dir <dir> …`.
- Multi-board runs build sequentially (each build already saturates cores);
  a failure names the board it failed on and continues-or-aborts → **aborts**
  (fail fast; CI runs one board per job anyway).
- Tests: flag wiring/validation and board resolution against a fake builder
  seam; no Docker in tests. `gosd build` behaviour is untouched (no changes to
  the build/run commands beyond `AddCommand`).

## Todos

- [x] Subcommand + flags + registration
- [x] Thread config/overlay + builder choice into kernelbuild
- [x] Success output points at `--artifacts-dir` usage
- [x] Behavioral tests with fake builder
- [x] Quality gates green



## Summary of Changes

- Added `cmd/gosd/buildkernel.go`: `gosd build-kernel` (`newBuildKernelCmd`),
  registered in `newRootCmd`. Flags: `--board` (repeatable, reuses `build.go`'s
  `resolveBoards` so default/unknown-board/qemu-virt behavior matches
  `gosd build` exactly), `-o/--output` (default `./gosd-artifacts/`),
  `--config` (defaults to `gosd-kernel.toml` in the working directory if
  present), `--builder` (`docker`/`podman`, validated, default auto-detect),
  `--staging`.
- Added `internal/kernelconfig`: the minimal v1 gosd-kernel.toml parser —
  `Parse` decodes `[kernel.<board-id>]` `fragment`/`patches` (BurntSushi TOML,
  same idiom as `internal/gosdtoml`), and `Config.Overlay(boardID, baseDir)`
  resolves those paths (relative to the config file's directory) into a
  `kernelbuild.Overlay`, reading the fragment file and glob-expanding+reading
  each patches entry. Bean gosd-hkp7 grows this package in place for the full
  schema (strict unknown-key rejection, `[kernel]` based-on/builder,
  `[[firmware]]`).
- `runBuildKernel` validates `--builder`, resolves boards, loads the overlay
  config, calls `container.Detect` once, then builds each board sequentially
  via `buildKernelsForBoards`, which aborts on the first failure (naming the
  board) and never touches boards after it. On success it prints a per-board
  fresh/cache-hit line and the `gosd build --artifacts-dir <dir> ...`
  follow-up.
- Testability: `buildKernelsForBoards` takes the container runtime and the
  `kernelbuild.Build`-shaped builder function as explicit parameters (same
  function-value-injection pattern as `compileForBoards` in `archbuild.go`),
  and a small locally-defined `containerRuntime` interface (mirroring
  `internal/kernelbuild`'s own unexported `runner` interface) lets tests
  supply a fake runtime — so `cmd/gosd/buildkernel_test.go` exercises flag
  defaults, builder validation, config loading/defaulting/error paths, output/
  staging/overlay threading, cache-hit reporting, sequential abort-on-failure,
  default-vs-explicit-qemu-virt board resolution, and the success summary
  text, all without Docker/Podman.
- `go test ./...`, `go vet ./...`, `gofmt -l .`, and `golangci-lint run ./...`
  (both native and `GOOS=linux`) are all clean. No changes to `build.go`/
  `run.go` beyond the one `AddCommand` line in `main.go`; COMPATIBILITY.md is
  unaffected (no board/feature status changed).
