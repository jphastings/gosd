---
# gosd-7jmj
title: 'kernelbuild: cache key omits Defconfig/Toolchain/make-target/source-path — a fix to those fields silently re-serves the stale cached kernel'
status: completed
type: bug
priority: high
created_at: 2026-07-31T07:52:15Z
updated_at: 2026-07-31T07:52:15Z
---

Found by review sweep `gosd-fuxs` (kernel/CI infra area), verified.

`cacheInputs` (internal/kernelbuild/cache.go:26-35) hashes Repo, Ref,
Image, Fragment, Patches, OverlayFragment, OverlayPatches, OutputNames —
the locked recipe from bean `gosd-x488`. But `buildScript()`
(internal/kernelbuild/script.go) also bakes `spec.Defconfig`,
`spec.Toolchain` (ARCH/CROSS_COMPILE/cross package),
`spec.KernelMakeTarget`, `spec.KernelSourcePath`, and every
`DTB.MakeTarget`/`DTB.SourcePath` into the generated script. None are in
the key. The recipe's doc comment lists only RequiredY and KBUILD_* pins
as intentional exclusions, so this looks like an oversight in the locked
recipe, not part of it — per project policy this bean says so rather than
silently diverging: **the gosd-x488 recipe proves incomplete in practice.**

**Failure scenario:** a developer fixes a wrong `KernelSourcePath` (wrong
DTB copied out of the tree) or corrects Defconfig/Toolchain without
touching ref/fragment/patches. Cache key unchanged → `Skipped: true` →
the old, wrong kernel/DTB is re-served forever until the cache dir is
manually cleared — a stale artifact shipped under a "fixed" code path.
`TestBuild_CacheMissesOnChangedInput` covers ref/fragment/overlay/DTB-list
changes but none of these fields (confirming the gap is untested).

**Fix:** simplest and most robust — hash the fully-rendered
`buildScript(spec)` output itself, which by construction captures every
field that affects the build; keep the intentional exclusions by
construction (they aren't in the script). Otherwise add the missing fields
to `cacheInputs` and extend the cache-miss test to cover each.

## Summary of Changes

Checked the "hash the whole script" option first, per the bean's own
caveat: `buildScript()` (script.go) calls `writeAssertions`, which bakes
the `RequiredY`/`ForbiddenY` lists straight into the generated script as
the post-`olddefconfig` assertion loop. Hashing the rendered script whole
would therefore silently pull `RequiredY`/`ForbiddenY` into the cache key
— exactly the documented intentional exclusion `cacheInputs`' doc comment
calls out. So this bean took the other branch: added the missing fields
to `cacheInputs` explicitly, rather than hashing the script.

`internal/kernelbuild/cache.go`'s `cacheInputs` now also hashes
`Defconfig`, `Toolchain.KernelArch`, `Toolchain.CrossCompile`,
`KernelMakeTarget`, `KernelSourcePath`, and each DTB's
`MakeTarget`/`SourcePath` (new `cacheDTB` shape, mirroring the existing
`cachePatch` pattern) — every field `buildScript` bakes into the generated
script that wasn't already keyed. `RequiredY`, `ForbiddenY`,
`ModulesDisabled` and the `Reproducibility`/`KBUILD_*` pins remain
excluded, now for a documented reason each: the first three are
post-build assertions rather than build inputs, and the KBUILD pins are
passed as container environment variables, never baked into the script
itself. The doc comment on `cacheInputs` records this — it explicitly
supersedes gosd-x488's original field list, since gosd-7jmj proved that
recipe incomplete in practice, and explains why hashing the whole script
was rejected.

`TestBuild_CacheMissesOnChangedInput` gained one case per newly-keyed
field (Defconfig, Toolchain arch, Toolchain cross-compile,
KernelMakeTarget, KernelSourcePath, DTB MakeTarget, DTB SourcePath), and a
new `TestBuild_CacheHitsDespiteRequiredYChange` pins the intentional
exclusion end-to-end: building once, then changing only
RequiredY/ForbiddenY and building again with the same cache dir, asserts
`Result.Skipped` stays true and the container is not re-run.

This changes every existing cache key once — anyone upgrading rebuilds
their cached kernels on their next `gosd build-kernel` run. One-time cost,
correct behavior: their old cache entries simply stop matching and a
fresh, complete build populates new ones.
