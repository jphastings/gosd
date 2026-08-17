# Artifact pipeline: cutting and consuming a GoSD artifact release

`gosd build` needs a kernel, DTB, and (for the Radxa Zero 3E, NanoPi Zero2,
ROCK 4SE, and Cubie A5E) a bootloader for every board it targets. GoSD
compiles these itself — it never asks a user to build a kernel — and ships
them as GitHub Releases tagged `artifacts/vX.Y.Z`, separate from the CLI's
own `vX.Y.Z` releases. This page covers cutting one of those releases, and
how the CLI consumes it. See bean `gosd-wtpa` for the design history.

Third-party binary blobs (Pi GPU firmware, Pi WiFi firmware, Rockchip rkbin)
are **not** part of an artifact release: they stay upstream-fetched by the
CLI at a pinned URL + sha256 per board (see each board's
`build/boards/<board>/manifest.json` and `internal/fetch`). An artifact
release contains only what GoSD compiles: kernels and U-Boot.

## What's in a release

Pushing a git tag `artifacts/vX.Y.Z` runs
`.github/workflows/build-artifacts.yml`, which:

1. Runs `gosd build-kernel --board <id> --staging staging/` (bean gosd-07fl)
   for each of `pi-zero-2w`, `pi-zero-w`, `pi-3b`, `radxa-zero-3e`,
   `nanopi-zero2`, `rock-4se`, `cubie-a5e`, and `qemu-virt` — one job per
   board, each driving Docker from `internal/kernelspec`'s declarative
   per-board spec, cross-compiling for arm64 (or, for pi-zero-w, armv6) via a
   `-linux-gnu-` cross toolchain, so they run unchanged on GitHub's amd64
   `ubuntu-latest` runners (no QEMU, no arm64 runner needed). This is the
   same command a developer runs locally with `gosd build-kernel` — CI
   dogfoods the real CLI path rather than a separate release-only script.
   `gosd build-kernel --staging` also writes each board's `source.json`
   directly, so the release path and the local dev path produce identical
   provenance data. The four U-Boot boards (radxa-zero-3e, nanopi-zero2,
   rock-4se, cubie-a5e) additionally run their own
   `build/boards/<board>/uboot/build.sh` — U-Boot orchestration is out of
   scope for `gosd build-kernel` (epic gosd-47rm) and stays a plain script; a
   small workflow step merges its pinned repo/tag into the board's
   already-written `source.json` (rock-4se and cubie-a5e also fold in their
   from-source Trusted Firmware-A provenance here), since the U-Boot script
   has no `source.json` of its own.
2. Packages the outputs into per-board tarballs — `pi-zero-2w.tar.zst`,
   `pi-zero-w.tar.zst`, `pi-3b.tar.zst`, `radxa-zero-3e.tar.zst`,
   `nanopi-zero2.tar.zst`, `rock-4se.tar.zst`, `cubie-a5e.tar.zst`, and
   `qemu-virt.tar.zst` — using `build/artifacts/package.sh`, which also
   writes a `manifest.json` describing every file's name, sha256, and size, plus each compiled
   component's source repo/commit-or-tag/config path (GPL provenance).
   `gosd build-kernel --staging` emits the generated `kernel.config`
   alongside the kernel image and DTB, so that file is packaged into the
   tarball too.
3. Uploads the tarballs and `manifest.json` to the GitHub Release that
   already exists for the pushed tag — knope creates that release, with its
   notes, when its release PR is merged (see
   [cutting a new release](#cutting-a-new-release) below); this workflow
   only attaches assets to it.

The workflow also has a `workflow_dispatch` trigger for testing the full
kernel-build → package pipeline on a branch without cutting a real release:
a dispatch run skips the final "Upload assets to the knope-published
release" step (tag-conditional on `refs/tags/artifacts/*`) and instead
uploads `dist/` as a workflow artifact for inspection.

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
  pi-3b/
    kernel8.img
    bcm2710-rpi-3-b.dtb
    bcm2710-rpi-3-b-plus.dtb
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
  rock-4se/
    Image
    rk3399-rock-4se.dtb
    kernel.config
    idbloader.img
    u-boot.itb
    source.json
  cubie-a5e/
    Image
    sun55i-a527-cubie-a5e.dtb
    kernel.config
    u-boot-sunxi-with-spl.bin
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

1. Land the kernel/U-Boot change on `main` (an `internal/kernelspec`,
   config-fragment, or U-Boot `build.sh` change, reviewed and merged like
   any other PR) together with a change file declaring `artifacts: minor`
   (or `patch`/`major` as appropriate) describing the change — release
   notes now come from these files rather than a hardcoded body; see
   [how change files drive a release](releasing.md). Still
   **without** bumping `internal/artifacts.Version` in the same PR — that
   bump is a later, separate step, once the tag exists — for the same
   reason as before: bumping to an unpublished tag turns the qemu
   boot-to-HTTP CI job red.
2. When knope opens its release PR listing the `artifacts` package, merging
   it is the deliberate, human release act (this amends the old "no
   automation pushes tags" rule: the merge is the decision, tagging and
   release creation are the mechanical consequence). Merging creates the
   `artifacts/vX.Y.Z` tag and a GitHub Release (knope names it like `artifacts X.Y.Z`)
   with notes assembled from the accumulated change files.
3. That tag push fires the `Build artifacts` workflow, exactly as before.
   On success it attaches `pi-zero-2w.tar.zst`, `pi-zero-w.tar.zst`,
   `pi-3b.tar.zst`, `radxa-zero-3e.tar.zst`, `nanopi-zero2.tar.zst`,
   `rock-4se.tar.zst`, `cubie-a5e.tar.zst`, `qemu-virt.tar.zst`, and
   `manifest.json` to the release knope already created (20–60 min) — the
   workflow no longer creates or describes the release itself, only
   attaches assets to it.
4. A follow-up PR bumping `internal/artifacts.Version` to the new tag —
   so newly-built `gosd` binaries pick it up — **opens itself**. Once the
   asset upload above succeeds, `.github/workflows/pin-artifacts-version.yml`
   rewrites the constant (splicing the release's own notes into its doc
   comment) and opens a draft PR (bean gosd-odx3). It is a normal CLI-code
   change, part of the *next* CLI `vX.Y.Z` release, not the artifact release
   itself.

   If that PR is missing — the release predates the automation, or the job
   failed — run the workflow by hand from the Actions tab with the version
   as its input (`v0.10.2`), or locally with
   `build/artifacts/pin-bump.sh v0.10.2` and open the PR yourself. The
   workflow refuses to pin a release whose assets aren't attached yet, and
   does nothing if the constant already names that version.
5. **The PR arrives as a draft on purpose**: verify the bump three ways
   before marking it ready for review, and record each in the bean:
   - **Clean-machine build** — fresh `HOME`, no `--board`/`--artifacts-dir`
     flags, so every public board's image comes from a real download of the
     new release.
   - **Offline re-run** — kill network access (e.g. a dead proxy) and
     rebuild; it must succeed entirely from the now-populated cache.
   - **Content spot-check** — confirm the released artifact actually
     carries the change, e.g. `dtc -I dtb -O dts` showing the newly enabled
     DT node.
6. Also check the cacerts pin is current —
   `.github/workflows/cacerts-pin-check.yml` runs this check on a schedule
   and files an issue when it's not (bean gosd-w6zc); no need to duplicate
   its logic here.

**Escape hatch:** hand-pushing a tag still works for emergencies —
`git tag artifacts/vX.Y.Z && git push origin artifacts/vX.Y.Z` — but you
must create the release *first*, with `gh release create artifacts/vX.Y.Z`,
before pushing (or re-running) the tag: the `Build artifacts` workflow only
uploads assets to an existing release now, so a hand-pushed tag with no
release behind it fails the upload step.

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

### The cache is self-bounding, not append-only (bean gosd-gdro)

Without any cleanup, every `internal/artifacts.Version` bump would leave its
predecessor's tree (tens of MiB) behind forever, alongside every past
`internal/cacerts.Pin`/`internal/cloudflaredpin` bump's cached file - none of
that content is ever revisited once gosd moves on to a new version/pin. To
keep the everyday cache bounded to roughly the current version's footprint,
`gosd build`/`gosd run` prune it after a build succeeds: `gosd/artifacts/`
keeps only the currently pinned `<version>` directory (every sibling
`vX.Y.Z` is removed), and `gosd/cacerts/`/`gosd/ingress/` keep only the
file(s) matching the currently pinned CA bundle/cloudflared binary. This is
cheap - it only ever deletes content gosd itself cached from a pinned
URL/release, all of it trivially re-downloadable - and never breaks an
offline re-run of the *same* pinned version, since that version's directory
is exactly what's kept. Pruning is best-effort (a failure is logged, never
a build failure) and is skipped entirely on an `--artifacts-dir` build,
which may not touch this cache at all.

This does **not** cover `gosd build-kernel`'s durable build-state cache
(content-addressed, opt-in, expensive to rebuild - see
`internal/kernelbuild`) or offer a manual `gosd cache` inspection/clean
command; both are tracked as follow-ups on bean gosd-gdro.

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
