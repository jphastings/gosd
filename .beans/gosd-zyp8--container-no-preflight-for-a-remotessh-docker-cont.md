---
# gosd-zyp8
title: 'container: no preflight for a remote/SSH docker context — the documented empty-bind-mount gotcha fails generically'
status: todo
type: task
priority: low
created_at: 2026-07-31T07:54:33Z
updated_at: 2026-07-31T07:54:33Z
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
