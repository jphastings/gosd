---
# gosd-2jwa
title: Add a 'gosd cache' subcommand (dir / size / clean)
status: todo
type: task
priority: low
created_at: 2026-08-08T19:50:36Z
updated_at: 2026-08-08T19:50:36Z
---

Follow-up to gosd-gdro. Convenience surface for the caches gosd keeps on a user's machine, now that they self-bound automatically: 'gosd cache dir' prints the path(s), 'gosd cache size' shows per-location sizes, 'gosd cache clean' removes the download caches, '--builds' also clears the durable build-kernel/build-external state (see the keep-last-N follow-up). Everyday growth is already solved by gosd-gdro's auto-prune, so this is manual control / discoverability, not the core fix. Reuse the existing path helpers: artifactCacheDir/caCertsCacheDir/ingressCacheDir/kernelFirmwareCacheDir (cmd/gosd) and kernelbuild/extbuild defaultBuildRoot.
