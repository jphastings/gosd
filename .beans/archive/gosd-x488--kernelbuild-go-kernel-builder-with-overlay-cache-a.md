---
# gosd-x488
title: 'kernelbuild: Go kernel builder with overlay, cache and provenance'
status: completed
type: task
priority: normal
created_at: 2026-07-11T07:41:32Z
updated_at: 2026-07-11T11:04:31Z
parent: gosd-47rm
blocked_by:
    - gosd-di6v
    - gosd-fe9w
---

Part of [[gosd-47rm]]. The Go kernel builder: given a board's `KernelSpec`
([[gosd-di6v]]) plus an optional developer overlay, run the build in a
container ([[gosd-fe9w]]) and emit artifacts.

## Locked decisions

- New `internal/kernelbuild` package. It generates the in-container build
  script from the KernelSpec — the same steps the retired shell scripts do
  today: shallow clone repo@ref → apply GoSD patches → `make <defconfig>` →
  `merge_config.sh` the GoSD fragment → apply **developer overlay** (extra
  fragment merged after ours; extra patches applied after ours) →
  `olddefconfig` → assert the required-`=y` list survived (fail with the
  missing symbols named) → `make Image` + DTB with the `KBUILD_BUILD_*` pins →
  copy outputs + the generated `.config` to the output mount.
- **Output contract** (what makes it drop-in):
  - a **flat directory** whose filenames equal the board's
    `ArtifactRef.Name`s — directly usable as `gosd build --artifacts-dir`
  - plus `source.json` (`{kernel: {repo, ref, config}}`, matching
    `internal/artifacts.ComponentSource`) for GPL provenance, and the generated
    `kernel.config` for inspection
  - a `--staging` style mode (exact flag lives on the CLI bean) arranging the
    same files as `staging/<board>/` so `build/artifacts/package.sh` consumes
    it unchanged in CI
- **Idempotence / cache:** key = hash(kernel ref, image digest, fragment,
  patches, overlay, KernelSpec outputs). On key match with outputs present,
  skip the build and say so. Cache under `os.UserCacheDir()/gosd/kernel-build/
  <key>/`. No half-written cache entries: build into temp, rename on success
  (same pattern as `internal/artifacts.ensureBoard`).
- Overlay type is defined here (fragment bytes + patches); TOML parsing of
  `gosd-kernel.toml` is the config bean's job — this package takes parsed
  values.
- Behavioral tests on macOS with a fake container runtime: script generation
  (steps, order, merge-after-ours overlay semantics), output naming per board,
  `source.json` contents, cache hit/miss, required-`=y` failure surfacing the
  symbol names. No real kernel compile in unit tests.

## Todos

- [x] Build-script generation from KernelSpec (+ overlay semantics)
- [x] Run via container runtime; stream logs
- [x] Flat-output + staging-output collection, `source.json`
- [x] Content-addressed cache with atomic rename
- [x] Fake-runtime behavioral tests
- [x] Quality gates green



## Summary of Changes

Added `internal/kernelbuild`, the Go-native kernel build orchestrator:

- `Build(ctx, spec, overlay, opts)` generates a bash script from a
  `kernelspec.KernelSpec` (+ optional developer `Overlay`), mirroring the
  retired `build/boards/*/build.sh`/`docker-build.sh` steps: shallow clone
  (commit-ref via init/fetch/checkout, tag-ref via `git clone --branch`),
  apply GoSD DTS patches, `make <defconfig>`, merge the GoSD fragment,
  apply the developer overlay (patches then fragment, both after GoSD's),
  `make olddefconfig`, assert `RequiredY`/`ForbiddenY` (normalizing both the
  Pi boards' `CONFIG_FOO=y`-shaped entries and the Rockchip-family boards'
  bare-symbol entries to one check), build the kernel image + DTB, copy
  outputs and the generated `.config` to `/out`. Runs via a small local
  `runner` interface satisfied by `*container.Runtime`, so tests inject a
  fake without changing `internal/container`.
- Content-addressed cache under `os.UserCacheDir()/gosd/kernel-build/<key>/`
  (key = hash of repo, ref, image, GoSD fragment/patches, overlay,
  output filenames); builds into a temp dir and `os.Rename`s into place only
  once every expected output is present, so an interrupted build leaves no
  cache entry. A cache hit skips the container run and reports
  `Result.Skipped`.
- `Outputs{FlatDir, StagingDir}` writes the board's artifact files (matching
  `ArtifactRef.Name`s) plus `kernel.config` and `source.json`
  (`internal/artifacts.ComponentSource` shape) flat and/or under
  `StagingDir/<BoardID>/`, matching what `build/artifacts/package.sh`
  expects.
- Tests are behavioral, using a fake `runner`: script step order and
  overlay-after-GoSD semantics, KBUILD env pins, cache hit/miss on every
  input (ref, fragment, overlay, image), no cache entry after an
  interrupted build, flat/staging output contents, `source.json` contents,
  and a `RequiredY` failure surfacing the missing symbol name in the
  returned error.

No real kernel compile is exercised anywhere (per the bean's boundary); no
real-container smoke test was added since it was optional in scope.
