---
# gosd-9o73
title: Bound the durable build-kernel/build-external cache (keep-last-N)
status: todo
type: task
priority: low
created_at: 2026-08-08T19:50:36Z
updated_at: 2026-08-08T19:50:36Z
---

Follow-up to gosd-gdro (which self-bounded the everyday download caches). The durable content-addressed build-state dir (internal/kernelbuild + internal/extbuild defaultBuildRoot: ~/Library/Application Support/gosd on macOS, $XDG_STATE_HOME elsewhere) still grows one large entry per distinct build input and never reclaims stale ones — measured at 652MiB on JP's Mac. It is opt-in (Docker driver-devs) and EXPENSIVE to rebuild (20-75min per kernel), so keep-current-only would be hostile: bound it by keep-last-N (or an age/size cap), pruning stale content-addressed entries only AFTER a successful build and never the just-built key. Preserve the gosd-l4y9 property that this dir is NOT under UserCacheDir (macOS must not purge it mid-build). Parent principle (JP, 2026-08-08): nothing may grow in proportion to how many times/versions of gosd are run.
