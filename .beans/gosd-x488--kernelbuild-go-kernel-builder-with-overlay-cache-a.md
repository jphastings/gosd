---
# gosd-x488
title: 'kernelbuild: Go kernel builder with overlay, cache and provenance'
status: todo
type: task
created_at: 2026-07-11T07:41:32Z
updated_at: 2026-07-11T07:41:32Z
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

- [ ] Build-script generation from KernelSpec (+ overlay semantics)
- [ ] Run via container runtime; stream logs
- [ ] Flat-output + staging-output collection, `source.json`
- [ ] Content-addressed cache with atomic rename
- [ ] Fake-runtime behavioral tests
- [ ] Quality gates green
