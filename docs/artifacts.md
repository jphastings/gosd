# Artifact pipeline: cutting and consuming a GoSD artifact release

`gosd build` needs a kernel, DTB, and (for the Radxa Zero 3E) a bootloader
for every board it targets. GoSD compiles these itself — it never asks a
user to build a kernel — and ships them as GitHub Releases tagged
`artifacts/vX.Y.Z`, separate from the CLI's own `vX.Y.Z` releases. This page
covers cutting one of those releases, and how the CLI consumes it. See bean
`gosd-wtpa` for the design history.

Third-party binary blobs (Pi GPU firmware, Pi WiFi firmware, Rockchip
rkbin) are **not** part of an artifact release: they stay upstream-fetched
by the CLI at a pinned URL + sha256 per board (see each board's
`build/boards/<board>/manifest.json` and `internal/fetch`). An artifact
release contains only what GoSD compiles: kernels and U-Boot.

## What's in a release

Pushing a git tag `artifacts/vX.Y.Z` runs
`.github/workflows/build-artifacts.yml`, which:

1. Runs `build/boards/pi-zero-2w/build.sh`, `build/boards/radxa-zero-3e/kernel/build.sh`,
   and `build/boards/radxa-zero-3e/uboot/build.sh` — each Dockerized, each
   cross-compiling for arm64 via `aarch64-linux-gnu-`, so they run unchanged
   on GitHub's amd64 `ubuntu-latest` runners (no QEMU, no arm64 runner
   needed).
2. Packages the outputs into two tarballs — `pi-zero-2w.tar.zst` and
   `radxa-zero-3e.tar.zst` — using `build/artifacts/package.sh`, which also
   writes a `manifest.json` describing every file's name, sha256, and size,
   plus each compiled component's source repo/commit-or-tag/config path
   (GPL provenance).
3. Publishes a GitHub Release for the pushed tag with the two tarballs and
   `manifest.json` attached.

`build/artifacts/package.sh` is a standalone script, runnable and testable
without Docker, a real kernel build, or network access — point it at any
staging directory laid out like:

```
staging/
  pi-zero-2w/
    kernel8.img
    bcm2710-rpi-zero-2-w.dtb
    source.json        # optional; copied into manifest.json verbatim
  radxa-zero-3e/
    Image
    rk3566-radxa-zero-3e.dtb
    idbloader.img
    u-boot.itb
    source.json
```

and run `build/artifacts/package.sh <version> staging <output-dir>` to get
the same tarballs + manifest.json the workflow publishes.

## Cutting a new release

1. Land whatever kernel/U-Boot changes are needed on `main` (a `build.sh` or
   `kernel.config`/config-fragment change, reviewed and merged like any
   other PR).
2. Decide the new version number (`vX.Y.Z`, independent of the CLI's own
   version — bump the artifact version when kernels/U-Boot change, not when
   unrelated CLI code changes).
3. Push a tag: `git tag artifacts/vX.Y.Z && git push origin artifacts/vX.Y.Z`.
4. Watch the `Build artifacts` workflow run. On success it publishes a
   GitHub Release named `Artifacts vX.Y.Z` with `pi-zero-2w.tar.zst`,
   `radxa-zero-3e.tar.zst`, and `manifest.json` attached.
5. Bump `internal/artifacts.Version` to `vX.Y.Z` in a follow-up commit (a
   normal CLI-code change, part of the *next* `vX.Y.Z` CLI release, not the
   artifact release itself) so newly-built `gosd` binaries pick it up.

Steps 3-4 are a manual, human step — no automation here pushes tags for you,
by design: cutting an artifact release is a deliberate decision, not a side
effect of merging a PR.

## How the CLI consumes a release: pinning and caching

`internal/artifacts.Version` is a constant naming the `artifacts/vX.Y.Z` tag
the current build of `gosd` downloads from. It's the *only* thing that
determines which kernels a `gosd build` run fetches — there is no
environment variable or flag to override it, so that every copy of a given
`gosd` binary behaves identically.

When `gosd build` needs a compiled artifact (e.g. `kernel8.img`) that isn't
found in `--artifacts-dir`, `internal/boards.ResolveArtifacts` falls back to
`internal/artifacts.EnsureBoard`, which:

1. Checks whether that board's files are already cached (see below) and
   still verify against a previously-cached `manifest.json`. If so, it
   returns immediately — no network request at all.
2. Otherwise, downloads `manifest.json` from the pinned release, then the
   requested board's `.tar.zst`, extracts it, and verifies every file the
   manifest lists against its sha256. Only once every file verifies is the
   result made visible in the cache (a corrupted or tampered download never
   contaminates it).

Cache location: `os.UserCacheDir()/gosd/artifacts/<version>/<board>/`, e.g.
`~/Library/Caches/gosd/artifacts/v0.1.0/pi-zero-2w/` on macOS or
`~/.cache/gosd/artifacts/v0.1.0/pi-zero-2w/` on Linux. Every `gosd` binary
pinned to the same `internal/artifacts.Version` shares this cache; a second
build (same or a different board) after the first successful one works
fully offline.

Failure modes are reported actionably rather than as a bare error chain:

- **Checksum mismatch** — a downloaded file doesn't match the manifest's
  pinned sha256 (corrupted transfer, or a tampered release). The download is
  rejected outright; nothing is cached, and the message says so.
- **Offline with no cache** — the manifest can't be downloaded and nothing
  is cached yet for that board. The error explains that either a working
  network connection is needed for the first build, or the artifacts can be
  supplied directly via `--artifacts-dir`.

## `--artifacts-dir` for local development

`--artifacts-dir <dir>` (see `internal/boards.ResolveArtifacts`) is checked
before any of the above, for every artifact a board needs — pass it a
directory containing your own `kernel8.img`/`Image`/etc. (e.g. output from
running a `build/boards/*/build.sh` locally) to iterate on a kernel change
without cutting a release. `cmd/gosd/testdata/fake-artifacts/` is the
placeholder set gosd's own test suite uses; it's wired in only via explicit
`--artifacts-dir` flags in tests, never as a default fallback in production
code.
