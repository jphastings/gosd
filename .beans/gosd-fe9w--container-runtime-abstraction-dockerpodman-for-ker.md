---
# gosd-fe9w
title: Container runtime abstraction (docker/podman) for kernel builds
status: todo
type: task
created_at: 2026-07-11T07:41:32Z
updated_at: 2026-07-11T07:41:32Z
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

- [ ] Package with Detect + Run over an exec seam
- [ ] Docker/podman detection incl. daemon liveness + precedence
- [ ] Digest-pinned image constant
- [ ] Actionable error taxonomy (not installed / daemon down / run failed)
- [ ] Fake-driven tests, green on macOS with no container runtime
- [ ] Quality gates green
