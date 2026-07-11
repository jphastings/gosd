# Artifact pipeline: cutting and consuming a GoSD artifact release

`gosd build` needs a kernel, DTB, and (for the Radxa Zero 3E and NanoPi
Zero2) a bootloader for every board it targets. GoSD compiles these itself —
it never asks a user to build a kernel — and ships them as GitHub Releases
tagged `artifacts/vX.Y.Z`, separate from the CLI's own `vX.Y.Z` releases.
This page covers cutting one of those releases, and how the CLI consumes
it. See bean `gosd-wtpa` for the design history.

Third-party binary blobs (Pi GPU firmware, Pi WiFi firmware, Rockchip
rkbin) are **not** part of an artifact release: they stay upstream-fetched
by the CLI at a pinned URL + sha256 per board (see each board's
`build/boards/<board>/manifest.json` and `internal/fetch`). An artifact
release contains only what GoSD compiles: kernels and U-Boot.

## What's in a release

Pushing a git tag `artifacts/vX.Y.Z` runs
`.github/workflows/build-artifacts.yml`, which:

1. Runs `gosd build-kernel --board <id> --staging staging/` (bean gosd-07fl)
   for each of `pi-zero-2w`, `pi-zero-w`, `radxa-zero-3e`, `nanopi-zero2`,
   and `qemu-virt` — one job per board, each driving Docker from
   `internal/kernelspec`'s declarative per-board spec, cross-compiling for
   arm64 (or, for pi-zero-w, armv6) via a `-linux-gnu-` cross toolchain, so
   they run unchanged on GitHub's amd64 `ubuntu-latest` runners (no QEMU, no
   arm64 runner needed). This is the same command a developer runs locally
   with `gosd build-kernel` — CI dogfoods the real CLI path rather than a
   separate release-only script. `gosd build-kernel --staging` also writes
   each board's `source.json` directly, so the release path and the local
   dev path produce identical provenance data. The two U-Boot boards
   (radxa-zero-3e, nanopi-zero2) additionally run their own
   `build/boards/<board>/uboot/build.sh` — U-Boot orchestration is out of
   scope for `gosd build-kernel` (epic gosd-47rm) and stays a plain script; a
   small workflow step merges its pinned repo/tag into the board's
   already-written `source.json`, since the U-Boot script has no `source.json`
   of its own.
2. Packages the outputs into per-board tarballs — `pi-zero-2w.tar.zst`,
   `pi-zero-w.tar.zst`, `radxa-zero-3e.tar.zst`, `nanopi-zero2.tar.zst`, and
   `qemu-virt.tar.zst` — using `build/artifacts/package.sh`, which also
   writes a `manifest.json` describing every file's name, sha256, and size,
   plus each compiled component's source repo/commit-or-tag/config path (GPL
   provenance). Because `gosd build-kernel --staging` also emits the
   generated `kernel.config` alongside the kernel image and DTB, that file is
   now packaged into the tarball too (previously only referenced by path from
   a committed copy) — a small, deliberate content change, not a regression.
3. Publishes a GitHub Release for the pushed tag with the tarballs and
   `manifest.json` attached.

The workflow also has a `workflow_dispatch` trigger for testing the full
kernel-build → package pipeline on a branch without cutting a real release:
a dispatch run skips the final "Publish GitHub Release" step (tag-conditional
on `refs/tags/artifacts/*`) and instead uploads `dist/` as a workflow
artifact for inspection.

`qemu-virt` is an **internal-only board**: it's a CI/local-dev boot-testing
profile (bean gosd-5wm0, epic gosd-c54j), never advertised in end-user docs
and excluded from the default all-boards `gosd build`. It's still packaged
into the same release as the public boards, purely so
`internal/artifacts.EnsureBoard` and local `--board=qemu-virt` builds can
fetch its kernel through the exact same cache/download path as any other
board — there is no separate distribution mechanism for it.

`build/artifacts/package.sh` is a standalone script, runnable and testable
without Docker, a real kernel build, or network access — point it at any
staging directory laid out like:

```
staging/
  pi-zero-2w/
    kernel8.img
    bcm2710-rpi-zero-2-w.dtb
    kernel.config
    source.json        # optional; copied into manifest.json verbatim
  pi-zero-w/
    kernel.img
    bcm2835-rpi-zero-w.dtb
    kernel.config
    source.json
  radxa-zero-3e/
    Image
    rk3566-radxa-zero-3e.dtb
    kernel.config
    idbloader.img
    u-boot.itb
    source.json
  nanopi-zero2/
    Image
    rk3528-nanopi-zero2.dtb
    kernel.config
    idbloader.img
    u-boot.itb
    source.json
  qemu-virt/
    Image
    kernel.config
    source.json
```

(`gosd build-kernel --staging staging/` produces exactly this per-board
layout, `kernel.config`/`source.json` included — see
`internal/kernelbuild/output.go`.)

and run `build/artifacts/package.sh <version> staging <output-dir>` to get
the same tarballs + manifest.json the workflow publishes.

## Cutting a new release

1. Land whatever kernel/U-Boot changes are needed on `main` (an
   `internal/kernelspec.go`, config-fragment, or U-Boot `build.sh` change,
   reviewed and merged like any other PR).
2. Decide the new version number (`vX.Y.Z`, independent of the CLI's own
   version — bump the artifact version when kernels/U-Boot change, not when
   unrelated CLI code changes).
3. Push a tag: `git tag artifacts/vX.Y.Z && git push origin artifacts/vX.Y.Z`.
4. Watch the `Build artifacts` workflow run. On success it publishes a
   GitHub Release named `Artifacts vX.Y.Z` with `pi-zero-2w.tar.zst`,
   `pi-zero-w.tar.zst`, `radxa-zero-3e.tar.zst`, `nanopi-zero2.tar.zst`,
   `qemu-virt.tar.zst`, and `manifest.json` attached.
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
directory containing your own `kernel8.img`/`Image`/etc. (e.g. the `-o`
output of `gosd build-kernel` run locally) to iterate on a kernel change
without cutting a release. `cmd/gosd/testdata/fake-artifacts/` is the
placeholder set gosd's own test suite uses; it's wired in only via explicit
`--artifacts-dir` flags in tests, never as a default fallback in production
code.
