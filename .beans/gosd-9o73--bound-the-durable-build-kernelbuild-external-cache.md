---
# gosd-9o73
title: Bound the durable build-kernel/build-external cache (keep-last-N)
status: completed
type: task
priority: low
created_at: 2026-08-08T19:50:36Z
updated_at: 2026-08-20T05:52:05Z
---

Follow-up to gosd-gdro (which self-bounded the everyday download caches). The durable content-addressed build-state dir (internal/kernelbuild + internal/extbuild defaultBuildRoot: ~/Library/Application Support/gosd on macOS, $XDG_STATE_HOME elsewhere) still grows one large entry per distinct build input and never reclaims stale ones — measured at 652MiB on JP's Mac. It is opt-in (Docker driver-devs) and EXPENSIVE to rebuild (20-75min per kernel), so keep-current-only would be hostile: bound it by keep-last-N (or an age/size cap), pruning stale content-addressed entries only AFTER a successful build and never the just-built key. Preserve the gosd-l4y9 property that this dir is NOT under UserCacheDir (macOS must not purge it mid-build). Parent principle (JP, 2026-08-08): nothing may grow in proportion to how many times/versions of gosd are run.

## Summary of Changes

Bounded both durable build-state dirs (`internal/kernelbuild`,
`internal/extbuild`) to their 8 most recently used content-addressed
entries, mirrored across the two sibling packages (matching this
codebase's existing pattern of duplicating kernelbuild/extbuild
infrastructure rather than sharing an abstraction between them — see
extbuild's package doc):

- New `cacheprune.go` in each package: `pruneStaleCacheEntries` lists
  `cacheKeyPattern`-shaped (hex-sha256) entries under the cache root, keeps
  the current build's key plus the `keepBuildCacheEntries` (8) most
  recently modified others, and `os.RemoveAll`s the rest. A `work-*`/
  `build.tmp-*` staging dir from an interrupted build (or any other
  unrecognised entry) is never touched — same "only remove what the code
  recognises the shape of" discipline as `cmd/gosd`'s existing
  `pruneCacheToCurrent`.
- `touchAndPruneCache` bumps the current entry's mtime to now (so a
  cache-hit-only board still counts as recently *used*, not just recently
  *built*) and then prunes, best-effort: a failure is written to
  `Options.Stderr` and otherwise ignored, since this only ever runs after
  `Build` has already produced a successful result.
- Wired into both packages' `Build` on both the cache-hit and freshly-built
  paths.
- 8 was chosen to comfortably cover the current board fleet's kernels (8
  registered boards including qemu-virt) plus a couple of in-flight overlay
  iterations, without growing in proportion to how long or how often
  `gosd build-kernel`/`build-external` has been run — the same principle
  gosd-gdro established for the download caches. Preserves gosd-l4y9's
  property unchanged: the build root stays out of `os.UserCacheDir()`.
- Tests: in-package `cacheprune_test.go` unit-tests the pruning/touch logic
  directly (keeps only the N most recent, never removes the current key,
  leaves unmanaged entries alone, no-ops on a missing dir); an
  external-package `TestBuild_BoundsCacheToTheMostRecentEntries` in each of
  `kernelbuild_test.go`/`extbuild_test.go` drives `Build` through 10
  distinct cache keys via a fake runner and asserts the cache directory
  never exceeds 8 entries and never loses the most recent one.
