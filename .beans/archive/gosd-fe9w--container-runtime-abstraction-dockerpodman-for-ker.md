---
# gosd-fe9w
title: Container runtime abstraction (docker/podman) for kernel builds
status: completed
type: task
priority: normal
created_at: 2026-07-11T07:41:32Z
updated_at: 2026-07-11T08:22:04Z
parent: gosd-47rm
---

Part of [[gosd-47rm]]. A small container-runtime abstraction so Go code can run
a build inside Docker or Podman. Nothing like this exists in the tree today —
all container use is bash (`build/boards/**/build.sh`).

## Locked decisions

- New `internal/` package (suggested name `internal/container`; implementer may
  refine) that shells out to the container CLI — **no Docker SDK dependency**
  (keeps the module light and works identically for podman).
- **Detection order:** honor an explicit choice first (`--builder` flag /
  config), else auto-detect `docker` then `podman` on `$PATH`, verifying the
  daemon actually responds (e.g. `docker info`), not just binary presence —
  colima/lima/Docker Desktop all look like `docker` and need no special-casing
  beyond a responding daemon.
- **Pin the build image by digest**, not a floating tag:
  `docker.io/library/debian:bookworm@sha256:…` (record the digest at
  implementation time). Reproducibility beats freshness; bumping the digest is
  a reviewed change.
- API surface (shape, not final signatures): `Detect(ctx, preferred string)
  (Runtime, error)`; `Runtime.Run(ctx, RunSpec{Image, Env, Mounts, Cmd,
  Stdout/Stderr})` streaming logs live (kernel builds run 20–60 min — the user
  must see progress).
- **Errors are actionable** per the CLI convention: no runtime →
  "gosd build-kernel needs Docker or Podman; install Docker Desktop
  (https://docs.docker.com/desktop/) or podman, then re-run"; daemon dead →
  say so and how to start it; distinguish the two.
- Behavioral tests on macOS without a daemon: injected fake exec seam (same
  pattern as gosd-init's platform seams); cover detection precedence, daemon-
  down vs not-installed errors, arg construction for mounts/env. A real-daemon
  smoke test may exist behind a skip-if-unavailable guard, but CI must not
  require a daemon for `go test ./...`.

## Todos

- [x] Package with Detect + Run over an exec seam
- [x] Docker/podman detection incl. daemon liveness + precedence
- [x] Digest-pinned image constant
- [x] Actionable error taxonomy (not installed / daemon down / run failed)
- [x] Fake-driven tests, green on macOS with no container runtime
- [x] Quality gates green



## Summary of Changes

Added `internal/container`: shells out to the `docker`/`podman` CLI (no SDK dependency), following gosd-init's platform-seam pattern — an `execRunner` interface (`LookPath`, `Run`) with a portable `os/exec` implementation (no build tags needed) and a fake used by all tests.

- `Detect(ctx, preferred)` honors an explicit preference, else tries docker then podman on $PATH; either way it runs a liveness check (`<runtime> info`) before returning a `*Runtime`, so a `docker` binary that resolves but whose daemon/VM isn't running is treated as unusable.
- `Runtime.Run(ctx, RunSpec)` shells out to `<runtime> run --rm` with Env/Mounts(+ro)/WorkDir/Cmd assembled into args (identical for docker and podman, since podman's CLI is docker-compatible for these flags), and streams stdout/stderr live via `os/exec`'s writer-copy behavior — no buffering, verified by a test that synchronizes on partial writes.
- `KernelBuildImage` pins `docker.io/library/debian:bookworm@sha256:30482e873082e906a4908c10529180aefb6f77620aea7404b909829fadc5d168`, obtained 2026-07-11 via `docker buildx imagetools inspect` locally and cross-checked against the Docker Hub API's top-level `digest` field for the `bookworm` tag (both agreed); the doc comment records how/when and that bumping it is a deliberate reviewed change.
- Three typed errors distinguish the failure modes: `*NotInstalledError` (with/without a specific Preferred runtime), `*DaemonDownError` (wraps the liveness-check error, hints how to start each runtime), and `*RunFailedError` (exit code + captured stderr tail, capped at 4KB).
- Tests are fake-driven and require no daemon; a separate `TestSmoke_DetectAndRun` exercises the real Detect+Run path (including an actual image pull) but only runs when a developer sets `GOSD_CONTAINER_SMOKE_TEST=1` — it stays off by default so CI's `go test ./...` (which runs on ubuntu-latest images that typically already have a live Docker daemon) never picks up an implicit network dependency.

Consumed by the future `internal/kernelbuild` (gosd-x488); nothing wires this package into a CLI command yet.
