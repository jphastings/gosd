---
# gosd-l4y9
title: kernel-build cache lives in macOS-evictable ~/Library/Caches; mid-build eviction kills 75-min builds
status: completed
type: bug
priority: normal
created_at: 2026-07-11T14:12:45Z
updated_at: 2026-07-11T14:31:46Z
parent: gosd-47rm
---

Found 2026-07-11 during the epic's real end-to-end verification, immediately after fixing [[gosd-0p21]]: a qemu-virt `gosd build-kernel` run compiled for ~75 minutes, then died at the copy-out step with `install: cannot create regular file '/out/Image': No such file or directory` — because the ENTIRE `~/Library/Caches/gosd/` tree had been recursively deleted on the host mid-build, out from under the live Docker bind mounts.

## Forensics

- Nothing in gosd removes the cache root (only per-entry `RemoveAll`s under it).
- The concurrently-running gosd-hkp7 agent audited its session: no deletes of the real cache (its cold-cache tests redirect HOME to t.TempDir()).
- The machine had ~12GiB free disk; a kernel build grows Docker's VM image by several GiB. macOS treats `~/Library/Caches` as purgeable under storage pressure (CacheDelete) — the most plausible deleter, and by contract apps must tolerate that directory vanishing at any time.

## Why this matters

Two things live under `os.UserCacheDir()/gosd/kernel-build` ([[gosd-x488]]'s locked location):
1. **In-flight build staging** (`work-*`, `build.tmp-*`) — eviction mid-build wastes an hour-plus and surfaces as a baffling container-side ENOENT.
2. **Completed cache entries** — 'cache' semantics say eviction is fine, but each entry costs 20–75 min to rebuild; treating them as casually evictable is a poor trade.

## Fix (locked-decision revision of gosd-x488's cache location, driven by field evidence)

Move the kernel-build root out of the evictable cache path to a durable, still-Docker-shared (under $HOME) state location:
- darwin: `~/Library/Application Support/gosd/kernel-build`
- linux: `$XDG_STATE_HOME/gosd/kernel-build` or `~/.local/state/gosd/kernel-build`
- fallback: `os.UserCacheDir()` behavior as today

Also: when a container run fails AND the staging dir has vanished from the host, say so actionably instead of surfacing the container's ENOENT.

## Summary of Changes

New `internal/kernelbuild/statedir.go`: `defaultBuildRoot()` resolves the
kernel-build root to a durable, Docker-shared state dir (darwin:
`~/Library/Application Support/gosd/kernel-build`; windows: UserConfigDir;
else `$XDG_STATE_HOME`/`~/.local/state`), replacing the evictable
`os.UserCacheDir()` default (gosd-x488's original location, revised here —
field evidence above). `Options.CacheDir` still overrides. When a container
run ends and the staging dir has vanished from the host, `Build` now returns
an explicit vanished-staging explanation instead of the container's ENOENT.
Tests cover per-OS resolution (injected goos/env) and both vanished-staging
paths (run-failed and run-succeeded).

## Todos

- [x] State-dir resolution helper (darwin/linux/fallback) with docstring explaining the eviction constraint
- [x] kernelbuild default root moves there; explicit CacheDir option still wins
- [x] Actionable error when staging dir disappears mid-build
- [x] Tests for resolution + vanished-staging error
- [x] Quality gates
