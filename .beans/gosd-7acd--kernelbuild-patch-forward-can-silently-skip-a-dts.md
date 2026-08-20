---
# gosd-7acd
title: 'kernelbuild: patch --forward can silently skip a DTS patch with no build-time verification it applied'
status: completed
type: task
priority: normal
created_at: 2026-07-31T07:53:52Z
updated_at: 2026-08-20T06:18:31Z
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

## Summary of Changes

Investigated the exact mechanism first (bean gosd-fuxs's finding was re-verified, not assumed): built the identical `docker.io/library/debian:bookworm`-family image the real kernel builds use and exercised `patch -p1 --forward` against several already-applied/reversed/fuzzy scenarios. Empirically, GNU patch 2.7.6/2.8 in this image already returns a nonzero exit in every scenario tried (full-hunk exact reverse, one-of-two-hunks already-applied, fuzzy-context mismatch) - so `set -euo pipefail` appears to already catch a plain skip today. That doesn't make the underlying concern moot though: `--forward`'s documented behavior is literally "ignore" a reversed/already-applied hunk, and the default fuzz factor (2) is a real, separate route to a hunk landing at a wrong, merely-similar-looking location - the man page's own words: "a larger fuzz factor increases the odds of a faulty patch."

Changed `writePatchLoop` (internal/kernelbuild/script.go): patches now apply with `--fuzz=0` (a hunk against a freshly cloned, exactly-pinned source tree can never legitimately need fuzzy matching - refusing it removes the ambiguous-match class entirely), dropped `--forward` (a synonym of `-N`, and keeping it read as tolerating exactly the outcome we don't want), and added an explicit check: patch's own combined output is captured and grepped for reversed/ignored/already-applied wording, failing loudly with a named FATAL line even if patch's exit code were ever 0 for such a case - defense in depth on top of `set -e`, not a replacement for it. Verified end to end against the real build image: a fresh patch applies and the loop reports "LOOP OK"; re-running against the now-already-patched tree fails immediately with the new FATAL message.

Added `TestScript_PatchApplicationFailsLoudlyOnAnyDeviation` (internal/kernelbuild/script_test.go) asserting the generated script contains `--fuzz=0`, the FATAL lines, and no longer contains `--forward`.
