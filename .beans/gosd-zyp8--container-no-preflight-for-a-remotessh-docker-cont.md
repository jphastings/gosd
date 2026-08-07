---
# gosd-zyp8
title: 'container: no preflight for a remote/SSH docker context — the documented empty-bind-mount gotcha fails generically'
status: completed
type: task
priority: low
created_at: 2026-07-31T07:54:33Z
updated_at: 2026-08-07T15:51:59Z
---

Found by review sweep `gosd-fuxs` (kernel/CI infra area), verified.

Detect/checkDaemon (internal/container/detect.go:45-78) only verify the
binary exists and `info` exits 0 — equally true for DOCKER_HOST=ssh://...
CLAUDE.md documents the failure plainly ("a remote/SSH docker context
mounts empty dirs and fails at once") but the user gets a generic
RunFailedError ("/work/build.sh: No such file or directory") with no hint.

**Fix:** preflight `docker context inspect`/DOCKER_HOST before runBuild;
on a non-local endpoint (ssh://, tcp:// non-loopback) fail early naming
the context and pointing at the run-where-daemon-and-repo-live-together
guidance. Matches the project's actionable-error rule.


## Summary of Changes

- `internal/container/preflight.go` (new): `checkLocalEndpoint` runs after
  `checkDaemon` inside `detect()`. It checks `DOCKER_HOST`
  (`CONTAINER_HOST` for podman) first, then falls back to
  `<binary> context inspect` for the active context's endpoint.
  `classifyEndpoint` parses the host URL: `unix://`/`npipe://` and
  loopback `tcp://`/`http(s)://` (127.0.0.1, ::1, localhost) pass as
  local; `ssh://` and non-loopback `tcp://`/`http(s)://` are remote.
  Anything it can't confidently classify (unparseable, no `context`
  subcommand support, malformed JSON) is treated as local so it can never
  false-positive block colima or a Podman build without context support —
  a false negative just falls back to today's generic failure.
- New error type `RemoteContextError` in preflight.go: names the command,
  runtime, the remote endpoint/context, and points at
  `docs/custom-kernels.md`/`docs/externals.md`'s "Supported hosts"
  sections; gives the concrete fix (`unset DOCKER_HOST` or
  `` `docker context use default` ``).
- `internal/container/container.go`: `execRunner` gained `LookupEnv`, so
  the preflight's env lookup is fake-driven like the rest of the package.
  `exec.go`/`fakes_test.go` implement it for `realExec` (`os.LookupEnv`)
  and `fakeExec` (an injectable map).
- `internal/container/detect.go`: wired `checkLocalEndpoint` into
  `detect()`'s per-candidate loop, right after `checkDaemon`; updated
  `Detect`'s doc comment.
- `internal/container/preflight_test.go` (new): fake-driven tests —
  colima-style local unix-socket context passes; a runtime without
  `context` support (Podman) isn't blocked; ssh:// and non-loopback
  tcp:// contexts fail with `*RemoteContextError`; `DOCKER_HOST`/
  `CONTAINER_HOST` short-circuit context inspection entirely and are
  checked per-engine; loopback `DOCKER_HOST` passes; malformed context
  JSON is treated as local; a `classifyEndpoint` table test; an
  `Error()`-message content test. No test requires a real docker/podman
  binary.
- `docs/custom-kernels.md` / `docs/externals.md`: added a paragraph to
  each "Supported hosts" section documenting the local-daemon requirement
  and that the CLI now refuses early on a remote context, so the error's
  doc pointer resolves to real content.

Scope kept to `internal/container` + its docs + tests per the task's
heads-up about a concurrent U-Boot session touching container call
sites — no call sites changed; `Detect`'s signature is unchanged.

**Gates:** `go test ./...`, `go vet ./...`, `gofmt -l .`,
`golangci-lint run ./...` and `GOOS=linux golangci-lint run ./...` all
clean. The machine was under heavy concurrent load from sibling agent
worktrees during this work: one unrelated `go test ./...` run hit a
10-minute CPU-contention timeout in `cmd/gosd`'s
`TestBuildProducesABootableImageFromFakeArtifacts` (passed cleanly in
23s re-run alone) and a full-suite rerun later hit `no space left on
device` on the shared disk (also once against golangci-lint's shared
cache dir, `~/Library/Caches/golangci-lint`, which isn't redirectable via
`GOCACHE`) — both are the documented shared-cache/disk-contention gotcha,
not a defect in this change; `internal/container`'s own tests passed
cleanly in every run, isolated or not.

## Todos

- [x] Read CLAUDE.md and internal/container's existing seams/test conventions
- [x] Implement remote-context/DOCKER_HOST preflight in internal/container
- [x] Keep colima's local unix-socket context passing; only genuinely
      remote endpoints refuse
- [x] Fake-driven tests for the classification logic, no docker required
- [x] Point the error at docs/custom-kernels.md and docs/externals.md;
      add the corresponding docs paragraph
- [x] Quality gates (go test/vet, gofmt, golangci-lint darwin + linux)
- [x] Update bean, open PR
