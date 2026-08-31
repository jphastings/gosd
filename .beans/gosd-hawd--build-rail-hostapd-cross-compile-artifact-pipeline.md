---
# gosd-hawd
title: 'Build rail: hostapd cross-compile + artifact pipeline'
status: todo
type: task
created_at: 2026-08-31T05:36:11Z
updated_at: 2026-08-31T05:36:11Z
parent: gosd-qfbk
---

WiFi AP epic gosd-qfbk bean 1. No dependency on the other child beans; blocks
bean 4 (build-time wiring needs the real artifact shape) and bean 6 (bench
verification needs the real binary).

## Locked decisions

- New arch-keyed build spec (mirrors `internal/kernelspec`'s declarative
  shape, but keyed on GOARCH not board — hostapd's build depends only on
  target arch, not per-board kernel config, so it's simpler than a kernel
  build: "compile once per GOARCH").
- Docker/Podman cross-compile via `internal/container`'s existing daemon-
  driving code, musl static build (per `docs/externals.md`'s own "prefer
  musl over glibc for anything nontrivial" guidance).
- Verify the result with `internal/staticelf.Verify` — the same check
  `--with-external`'s build already runs.
- Ships as one more file inside each applicable board's **existing**
  `internal/artifacts` per-board tarball, reusing `manifest.json`/
  `ManifestSHA256`, `EnsureBoard`'s download/cache/verify, and the existing
  tag-first-bump-second `artifacts/vX.Y.Z` release procedure — NOT a new
  release channel and NOT the pinned-URL-and-sha256 third-party-blob
  pattern (no prebuilt static hostapd exists for these targets the way
  GPU/WiFi firmware does).
- GPL-2 provenance (source repo, commit, build config) recorded in
  `manifest.json`, the same pattern already proven for the kernel.
- **arm64 only for v1** (epic decision 5) — fold into `build-artifacts.yml`
  for arm64 boards only.
- Open question (see epic bean, item 2): hostapd's GPL-2 license in this
  compiled-and-redistributed channel is architecturally identical to the
  kernel's already-solved GPL story, but is a new *kind* of artifact in
  that channel — flag for explicit sign-off before merging, don't assume.

## Todos

- [ ] Arch-keyed hostapd build spec + Docker recipe (musl static)
- [ ] `internal/staticelf.Verify` wired into the build/CI job
- [ ] Fold into the existing per-board `internal/artifacts` manifest/tarball
- [ ] GPL-2 provenance recorded in `manifest.json`
- [ ] Ship in the next `artifacts/vX.Y.Z` release per the tag-first,
      bump-second procedure (docs/artifacts.md) — do NOT bump
      `internal/artifacts.Version` in this PR
- [ ] Pre-merge-test the artifacts CI job via `workflow_dispatch` on the PR
      branch before the tag exists
