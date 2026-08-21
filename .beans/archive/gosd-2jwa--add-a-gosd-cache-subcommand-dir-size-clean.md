---
# gosd-2jwa
title: Add a 'gosd cache' subcommand (dir / size / clean)
status: completed
type: task
priority: low
created_at: 2026-08-08T19:50:36Z
updated_at: 2026-08-20T05:51:31Z
---

Follow-up to gosd-gdro. Convenience surface for the caches gosd keeps on a user's machine, now that they self-bound automatically: 'gosd cache dir' prints the path(s), 'gosd cache size' shows per-location sizes, 'gosd cache clean' removes the download caches, '--builds' also clears the durable build-kernel/build-external state (see the keep-last-N follow-up). Everyday growth is already solved by gosd-gdro's auto-prune, so this is manual control / discoverability, not the core fix. Reuse the existing path helpers: artifactCacheDir/caCertsCacheDir/ingressCacheDir/kernelFirmwareCacheDir (cmd/gosd) and kernelbuild/extbuild defaultBuildRoot.

## Summary of Changes

Added `gosd cache` (`cmd/gosd/cache.go`) with three subcommands, reusing
the existing path helpers named in this bean plus two new exported
accessors (`kernelbuild.BuildRoot()`, `extbuild.BuildRoot()` — their
`defaultBuildRoot` was previously unexported):

- `gosd cache dir` — prints every cache location's path.
- `gosd cache size` — walks each location and prints a human-readable size
  per location plus a total.
- `gosd cache clean` — deletes the four pinned-download caches (board
  artifacts, CA bundle, ingress binaries, kernel firmware) outright; every
  one is a sha256-verified download the next build/run re-fetches
  transparently, so this is always safe. It deliberately does NOT touch the
  durable build-kernel/build-external state dir — see gosd-9o73, whose
  keep-last-N bound this command relies on for everyday growth. `--builds`
  opts into also deleting that expensive state (20-75 min/entry to
  rebuild), with its own warning line printed before and after.

Tests in `cmd/gosd/cache_test.go` cover all three subcommands, including the
specific behavior this bean flagged as worth getting right: `clean` without
`--builds` must leave the build-kernel/build-external cache untouched, and
`--builds` must remove it.
