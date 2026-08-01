---
# gosd-7acd
title: 'kernelbuild: patch --forward can silently skip a DTS patch with no build-time verification it applied'
status: todo
type: task
priority: normal
created_at: 2026-07-31T07:53:52Z
updated_at: 2026-07-31T07:53:52Z
---

Found by review sweep `gosd-fuxs` (kernel/CI infra area), verified.

`writePatchLoop` (internal/kernelbuild/script.go:175-183) runs
`patch -p1 --forward` per patch; `--forward` exits 0 when it judges a hunk
"already applied" and skips it, so `set -euo pipefail` never fires. The
Kconfig side gets real post-build assertions (RequiredY/ForbiddenY vs the
generated .config); the DT side has nothing confirming a patch's effect
reached the built DTB.

**Failure scenario:** a fleet kernel-tag bump shifts .dts context enough
that fuzzy matching misjudges a Rockchip peripheral-enablement patch (or
pi-zero-w's dma-ranges patch, gosd-1ey5) as already applied and skips it.
Build passes, kernel caches and ships without the hardware enablement;
surfaces only at bring-up.

**Fix:** drop --forward in favour of `--fuzz=0 -N` with explicit failure
on skip (a fresh clone can never legitimately be "already applied"), or
add DT-side assertions mirroring writeAssertions (dtc-decompile the built
DTB, grep for each patch's expected node/status).
