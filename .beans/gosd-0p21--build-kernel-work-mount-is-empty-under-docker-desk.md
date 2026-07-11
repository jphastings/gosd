---
# gosd-0p21
title: build-kernel /work mount is empty under Docker Desktop (work dir in unshared $TMPDIR)
status: completed
type: bug
priority: normal
created_at: 2026-07-11T12:29:45Z
updated_at: 2026-07-11T12:41:22Z
---

Found 2026-07-11 during the epic's first real end-to-end run (`gosd build-kernel --board=qemu-virt` against local Docker Desktop): the container exits 127 with `bash: /work/build.sh: No such file or directory`.

## Cause

`internal/kernelbuild.runBuild` creates its work dir with `os.MkdirTemp("", …)`, which on macOS lands under `$TMPDIR` (`/var/folders/…`). Docker Desktop's VM only shares `/Users`, `/Volumes`, `/private`, `/tmp` by default, so the bind mount appears **empty** inside the container — the generated build script never arrives. The output mount worked because it already lives under the cache root (`~/Library/Caches/gosd/kernel-build`, inside `/Users`). The retired shell scripts never hit this: they always mounted the repo checkout.

Part of epic [[gosd-47rm]]; bug in [[gosd-x488]]'s merged implementation. The fake-runtime tests could not have caught a host-VM file-sharing behavior — this is exactly what the pre-[[gosd-07fl]] real-Docker verification was for.

## Fix

Create the work dir under the (already-created) cache root alongside the temp output dir, so both bind mounts live in a Docker-Desktop-shared location. Regression test asserts the /work mount's HostPath is under the cache root, with a comment explaining the sharing constraint.

## Summary of Changes

`internal/kernelbuild.runBuild` now creates its /work dir with
`os.MkdirTemp(cacheRoot, "work-*")` instead of `os.MkdirTemp("", …)`, with a
comment explaining the Docker Desktop file-sharing constraint. Regression
test `TestBuild_WorkDirLivesUnderCacheDir` pins the mount's HostPath under
the cache dir. Verified against real Docker Desktop: the previously-failing
`gosd build-kernel --board=qemu-virt` now enters the container build
(toolchain install + clone + compile underway).

## Todos

- [x] Move work dir under cacheRoot with an explanatory comment
- [x] Regression test on the /work mount HostPath
- [x] Re-run the real qemu-virt e2e to confirm the container starts building
