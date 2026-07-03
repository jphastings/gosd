---
# gosd-wtpa
title: 'Artifact pipeline: CI kernel/bootloader builds, versioned releases, CLI download+cache'
status: todo
type: task
priority: normal
created_at: 2026-07-02T21:10:00Z
updated_at: 2026-07-02T21:17:59Z
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

- [ ] Workflow + release automation (triggered by pushing an artifacts/v* tag)
- [ ] internal/artifacts fetch/verify/cache + unit tests (httptest server, corrupted-checksum case)
- [ ] Wire board profiles ArtifactRef to the manifest names; delete fake-artifact stubs from the default path
- [ ] Document cutting a new artifact release in docs/artifacts.md

## Acceptance
On a clean machine with network: `gosd build ./examples/hello --board=pi-zero-2w -o x.img` works with no flags; second run works offline.

## Decision note (2026-07-02)
Releases contain ONLY artifacts we compile (kernels, U-Boot) — third-party blobs are fetched from upstream by the CLI per board manifests. For GPL compliance every released kernel/U-Boot records source repo + commit + full config in manifest.json, and release notes link to them.
