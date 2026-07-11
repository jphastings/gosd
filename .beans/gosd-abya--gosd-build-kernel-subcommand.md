---
# gosd-abya
title: gosd build-kernel subcommand
status: todo
type: feature
created_at: 2026-07-11T07:41:32Z
updated_at: 2026-07-11T07:41:32Z
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

- [ ] Subcommand + flags + registration
- [ ] Thread config/overlay + builder choice into kernelbuild
- [ ] Success output points at `--artifacts-dir` usage
- [ ] Behavioral tests with fake builder
- [ ] Quality gates green
