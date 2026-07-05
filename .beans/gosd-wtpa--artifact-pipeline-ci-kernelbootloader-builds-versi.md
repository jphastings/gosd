---
# gosd-wtpa
title: 'Artifact pipeline: CI kernel/bootloader builds, versioned releases, CLI download+cache'
status: in-progress
type: task
priority: normal
created_at: 2026-07-02T21:10:00Z
updated_at: 2026-07-05T05:30:23Z
parent: gosd-y0x3
blocked_by:
    - gosd-70b2
    - gosd-eu2x
    - gosd-d458
    - gosd-c7tk
---

Prebuilt board artifacts so no gosd user ever compiles a kernel.

- GitHub Actions workflow `build-artifacts.yml`: runs each build/boards/*/build.sh (they are already Dockerized), collects outputs per board
- Release scheme (locked): git tags `artifacts/v0.X.Y` publish a GitHub Release containing per-board tarballs (`pi-zero-2w.tar.zst`, `radxa-zero-3e.tar.zst`) + a top-level `manifest.json`: `{version, boards: {<name>: {files: [{name, sha256, size}]}}}`
- CLI side `internal/artifacts`: each gosd release pins an artifact version constant; `gosd build` downloads the tarball for the selected board to `os.UserCacheDir()/gosd/artifacts/<version>/<board>/` on first use, verifies every sha256, then works fully offline. `--artifacts-dir` override (already exists) bypasses download for development
- Clear failure message on checksum mismatch or offline-without-cache

- [x] Workflow + release automation (triggered by pushing an artifacts/v* tag)
- [x] internal/artifacts fetch/verify/cache + unit tests (httptest server, corrupted-checksum case)
- [x] Wire board profiles ArtifactRef to the manifest names; delete fake-artifact stubs from the default path
- [x] Document cutting a new artifact release in docs/artifacts.md

## Acceptance
On a clean machine with network: `gosd build ./examples/hello --board=pi-zero-2w -o x.img` works with no flags; second run works offline.

## Decision note (2026-07-02)
Releases contain ONLY artifacts we compile (kernels, U-Boot) — third-party blobs are fetched from upstream by the CLI per board manifests. For GPL compliance every released kernel/U-Boot records source repo + commit + full config in manifest.json, and release notes link to them.

## Summary of Changes

- `.github/workflows/build-artifacts.yml`: new workflow, triggered by pushing
  a git tag matching `artifacts/v*`. Builds `build/boards/pi-zero-2w/build.sh`,
  `build/boards/radxa-zero-3e/kernel/build.sh`, and
  `build/boards/radxa-zero-3e/uboot/build.sh` (each already Dockerized and
  cross-compiling via `aarch64-linux-gnu-`/`crossbuild-essential-arm64` —
  confirmed none need an arm64 host or QEMU), records each compiled
  component's source repo/commit-or-tag/config path, packages the outputs
  via the new `build/artifacts/package.sh` into `pi-zero-2w.tar.zst` +
  `radxa-zero-3e.tar.zst` + `manifest.json` (locked schema:
  `{version, boards: {<name>: {source, files: [{name, sha256, size}]}}}`,
  `source` additive to the locked shape for GPL provenance), and publishes a
  GitHub Release via `softprops/action-gh-release`. Linted clean with
  `actionlint`.
- `build/artifacts/package.sh`: standalone, testable outside CI — tars a
  staging directory per board, computes sha256+size per file, merges in an
  optional `source.json`. Verified locally end-to-end: ran it against
  hand-built fake staging dirs, then pointed `internal/artifacts`'s core
  resolver at a plain file server serving its real output and confirmed both
  boards download/extract/verify cleanly.
- `internal/artifacts` (new package): `Version` constant pinning the
  `artifacts/<Version>` release tag (currently `v0.1.0`, not yet published —
  see below); `EnsureBoard` downloads `manifest.json` + a board's
  `.tar.zst` from this repo's GitHub Releases, verifies every file's sha256,
  and caches the result under `cacheDir/<Version>/<board>/`; later calls
  read the cached manifest and verify locally with zero network requests.
  Reuses `internal/fetch` by exporting its sha256-of-file helper
  (`fetch.SHA256File`) rather than duplicating it. Unit tests (httptest)
  cover the happy path, a corrupted/tampered tarball (checksum mismatch,
  actionable error, nothing cached), offline-with-cache (second call makes
  zero requests), offline-without-cache (actionable error mentioning
  `--artifacts-dir`), an unknown board, and a path-escaping tar entry.
- `internal/boards.ResolveArtifacts` gained a `board string` parameter and a
  `BoardArtifactsFunc` fallback parameter: for any `ArtifactRef` with no
  per-file URL that isn't found in `--artifacts-dir`, it now calls the
  fallback (memoized — at most one download per `ResolveArtifacts` call,
  however many such refs a board has) instead of immediately erroring.
  `internal/pipeline.Assemble` wires this to `artifacts.EnsureBoard`.
  Confirmed `cmd/gosd/testdata/fake-artifacts` was never used as a default
  fallback in production code (only via explicit `--artifacts-dir` in
  tests), so there was nothing to delete there.
- `docs/artifacts.md`: new doc covering what's in a release, cutting one
  (tag push), how CLI pinning/caching/offline behavior works, and
  `--artifacts-dir` for local dev.
- `.github/workflows/ci.yml` and `README.md` touched only to keep them
  honest about what's wired up now (see gosd-2ge8 for the ci.yml details).

**Honesty about what's unverified**: I cannot push tags or trigger GitHub
Actions from here. `build-artifacts.yml` is actionlint-clean and its
packaging/manifest step is exercised standalone as described above, but the
workflow itself has never actually run. The acceptance criterion ("on a
clean machine with network, `gosd build` works with no flags; second run
offline") needs a real published `artifacts/v0.1.0` release, which doesn't
exist yet — that's the one remaining manual step (push the tag), owned by
JP. `internal/artifacts.Version` is set to `v0.1.0` in anticipation of it;
every code path that reaches GitHub is exercised instead by this package's
httptest-backed tests. Bean stays in-progress until that release exists and
the acceptance criterion is confirmed against it.
