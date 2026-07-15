# Externals: bundling a cross-compiled companion binary

Not every companion process your app needs is a pure-Go library you can
`go get` and cross-compile alongside it. A hardware-accelerated video
player, a vendor CLI, or any other prebuilt executable your app supervises
over `os/exec` still needs to exist as a fully static `linux/arm64` (or
`linux/arm`, GOARM 6) binary before `gosd build` can embed it — and that
binary usually has its own, non-Go build system (autotools, CMake, a
vendor Makefile) that needs its own cross-toolchain.

`gosd build-external` is that build step: it reads a developer-authored
`gosd-external.toml` recipe naming a build script, runs that script inside a
Docker/Podman container with a cross-compiler on `$PATH`, verifies the
result is a fully static ELF binary matching the target arch, and writes it
to `./gosd-externals/<arch>/<name>` — ready for `gosd build
--with-external` to bundle into an image. (The driving use case is a
separate, unreferenced project — a video-playback appliance bundling a
static `mpv`, supervised by the app over mpv's JSON IPC — not shipped as an
in-repo example; see the "GPL/licensing carve-out" section below for why.)

## Two commands, two concerns

- **`gosd build-external`** cross-compiles. You own the build script; this
  command owns running it reproducibly in a container and verifying its
  output. See ["`gosd-external.toml` reference"](#gosd-externaltoml-reference)
  below.
- **`gosd build --with-external <path>[:<dest>]`** bundles. It takes any
  prebuilt static binary — one `gosd build-external` just produced, or one
  from anywhere else — and embeds it into the image's initramfs. See
  [`docs/runtime.md`'s "Bundling a companion binary"
  section](runtime.md#bundling-a-companion-binary---with-external) for its
  flag, dest defaulting/collision rules, and the "your app owns it at
  runtime" supervision contract (gosd-init stays a single-child supervisor;
  it never launches or restarts an external itself).

These are deliberately independent: `--with-external` never requires
Docker, so a binary built once (locally, in CI, by someone else entirely)
can be bundled repeatedly with zero container runtime on the machine
running `gosd build`.

## Quickstart

`gosd-external.toml` in your project root:

```toml
[external.mpv]
script = "build-mpv.sh"
arch   = ["arm64"]

[[external.mpv.source]]
name    = "mpv"
repo    = "https://github.com/mpv-player/mpv"
ref     = "v0.38.0"
license = "GPL-2.0-or-later"
```

`build-mpv.sh`, next to it — pinning and building mpv itself is entirely
your script's job; gosd only runs it and checks what comes out:

```sh
#!/usr/bin/env bash
set -euo pipefail
# $GOSD_ARCH ("arm64"/"arm-6"), $GOSD_CROSS_COMPILE (e.g.
# "aarch64-linux-gnu-") and $GOSD_OUTPUT (the exact path to write the
# binary to) are already set - see "The container contract" below. The
# image ships no cross-compiler preinstalled, so install one first:
apt-get update -qq && apt-get install -y -qq crossbuild-essential-arm64
git clone --branch v0.38.0 --depth 1 https://github.com/mpv-player/mpv /tmp/mpv
cd /tmp/mpv
# ... configure/build with $GOSD_CROSS_COMPILE, statically linked ...
cp build/mpv "$GOSD_OUTPUT"
```

```sh
gosd build-external
# -> ./gosd-externals/arm64/mpv (+ ./gosd-externals/arm64/mpv.source.json)

gosd build . --board pi-zero-2w --with-external ./gosd-externals/arm64/mpv -o hello.img
```

`gosd build-external` requires a local Docker or Podman daemon running
(Docker Desktop, [colima](https://colima.run/) in its default
docker-runtime mode, or podman) — it drives it directly
(`internal/container`), the same way `gosd build-kernel` does, auto-detecting
whichever one it finds unless `--builder` or the recipe's own
`[external.<name>].builder` says otherwise. This is an explicit, opt-in
exception to GoSD's usual "no build step needs Docker" rule (see
`CLAUDE.md`'s locked decisions) — `gosd build` itself still never requires a
container runtime; only `gosd build-kernel` and `gosd build-external` do,
and both say so in their own `--help` text and errors.

## The container contract

Your script runs as `/work/script.sh` inside the container, with three
environment variables already set:

| Variable | Meaning |
| --- | --- |
| `GOSD_ARCH` | The recipe's own arch token, e.g. `arm64` or `arm-6` — unambiguous, unlike a bare `GOARCH=arm` (which doesn't say GOARM). |
| `GOSD_CROSS_COMPILE` | The `CROSS_COMPILE`-style toolchain prefix to use once your script has a matching cross-compiler on `$PATH`, e.g. `aarch64-linux-gnu-` for arm64, `arm-linux-gnueabihf-` for arm-6 — drop it straight into an autotools `--host=`, a CMake toolchain file, or a Makefile's `$(CROSS_COMPILE)gcc`. The default image (plain Debian bookworm) ships **no cross-compiler preinstalled** — your script installs its own, e.g. `apt-get install -y crossbuild-essential-arm64` (arm64) / `crossbuild-essential-armhf` (arm-6), the same packages `gosd build-kernel`'s generated script installs, or a musl toolchain (see below). |
| `GOSD_OUTPUT` | The exact path (`/out/<name>`) your script must write the finished binary to. |

Your script must leave a file at `$GOSD_OUTPUT` when it exits 0; a
generated wrapper checks this immediately and fails the build loudly,
inside the container log, if it didn't.

## The fully-static-binary contract

GoSD's initramfs ships **no `ld.so` and no library layout** — there is
nowhere for a dynamic loader to resolve `.so` dependencies against, on
purpose (it keeps the image small and the boot path simple). Every
external must therefore be a **fully static** binary: no `PT_INTERP`
program header, and its ELF class/machine must match the arch it was built
for.

`gosd build-external` enforces this itself, right after your script exits
and before the result ever reaches the output directory or the cache: it
opens `$GOSD_OUTPUT`, confirms it parses as ELF, confirms its class/machine
matches the arch you asked for, and confirms it has no `PT_INTERP` header.
Any of those failing fails the build with an actionable error — you find
out at `gosd build-external` time, not three steps later when `gosd build
--with-external` (which re-checks the same properties against your
`--board`) or, worse, the booted device rejects it.

Getting a real static binary out of an upstream build system is usually the
hard part, not the GoSD side:

- **Prefer musl over glibc for anything nontrivial.** glibc's static
  linking support is officially unsupported upstream for NSS-using code
  (DNS, `getpwnam`, etc.) and `-static` frequently either fails to link or
  produces a binary that isn't actually fully static despite the flag. A
  musl cross toolchain (e.g. [musl.cc](https://musl.cc/)'s prebuilt
  `aarch64-linux-musl-cross` / `armv6-linux-musleabihf-cross` tarballs, or
  building your own with
  [musl-cross-make](https://github.com/richfelker/musl-cross-make)) sidesteps
  this entirely and is the path of least resistance for most C/C++ media
  and vendor codebases. Point `GOSD_CROSS_COMPILE`-derived tooling at the
  musl toolchain instead of the glibc one your container image ships.
- **Go binaries:** `CGO_ENABLED=0 GOOS=linux GOARCH=$GOARCH go build` is
  already fully static; no special linking flags needed. (If your Go
  external needs cgo for a C dependency, you're back in the C static-linking
  problem above — cross-compile the C library with the musl toolchain
  first.)
- **C/C++ via autotools/CMake:** `--enable-static --disable-shared` (or the
  CMake equivalent) plus `LDFLAGS=-static`, built against the musl
  toolchain, is the usual combination. Some build systems still leave one
  or two shared deps linked in even with `-static` passed — the
  `PT_INTERP` check will catch it if so; re-check `ldd`-equivalent output
  (`file` reporting "statically linked", or a bare
  `$CROSS_COMPILE readelf -d` showing no `NEEDED` entries) before assuming
  a build is done.

## GPL/licensing carve-out

GoSD **never redistributes** anything `gosd build-external` produces — it
has no artifact release channel for externals the way it does for its own
kernels/U-Boot (`docs/artifacts.md`). Every external is compiled locally,
by you, from sources your own recipe pins and clones (typically via `git
clone` in your build script). This mirrors the same carve-out
`docs/custom-kernels.md` already documents for `gosd build-kernel`.

That's also why `[[external.<name>.source]]` entries exist:

```toml
[[external.mpv.source]]
name    = "mpv"
repo    = "https://github.com/mpv-player/mpv"
ref     = "v0.38.0"
license = "GPL-2.0-or-later"
```

GoSD itself never clones, verifies, or otherwise touches these — they are
**provenance-only, recorded as-is** into `source.json` alongside each
built binary. If you distribute images bundling a GPL (or other
source-availability-obligated) external, you carry the same obligations
GoSD already handles for you on its own kernel artifacts: keep
`source.json` (or the repo/ref/license info in it) available alongside
anything you ship. An entry with a missing `repo`, `ref`, or `license` is
rejected by `gosd-external.toml` parsing before any build runs, since a
provenance record missing any of those defeats the point of recording it.

## Output layout

```
./gosd-externals/
├── arm64/
│   ├── mpv                  # the binary — hand this path to --with-external
│   └── mpv.source.json      # its provenance record
└── arm-6/
    ├── mpv
    └── mpv.source.json
```

Output is keyed by **arch, not board** — `arm64` covers pi-zero-2w,
radxa-zero-3e, and nanopi-zero2 alike; `arm-6` covers pi-zero-w — since an
external's toolchain and static-linking result depend only on the target
arch, not which board eventually boots it. The `source.json` filename is
prefixed with the external's own name (`<name>.source.json`, not a bare
`source.json`) because a single `<arch>/` directory can hold more than one
external's output side by side; a bare `source.json` would silently
overwrite between them.

Builds are content-addressed and cached (script bytes + container image +
arch + output name), mirroring `gosd build-kernel`: an unchanged recipe
re-run is an instant cache hit, reported as such in `gosd build-external`'s
summary line rather than re-running the container.

## `gosd build-external` flags

| Flag | Default | Meaning |
| --- | --- | --- |
| `--config` | `./gosd-external.toml` | The recipe to build from. |
| `--name` | (all) | Repeatable. Which `[external.<name>]` recipe(s) to build; omit to build every recipe the config declares. |
| `--arch` | (every declared arch) | Repeatable. Restricts which arch(es) to build. Each selected recipe builds the intersection of `--arch` and its own declared `arch = [...]`; requesting an arch that matches none of a selected recipe's declared arches is an error naming that recipe (almost always a typo or the wrong `--name`/`--arch` pairing). |
| `--builder` | (auto-detect) | `docker` or `podman`. Overrides both auto-detection and any recipe's own `[external.<name>].builder`. |
| `-o`, `--output` | `./gosd-externals/` | Output directory; see "Output layout" above. |

## `gosd-external.toml` reference

```toml
[external.<name>]                # one section per external, keyed by name
script  = "path/to/build.sh"     # required; resolved relative to this file
arch    = ["arm64", "arm-6"]     # required, at least one: arm64 and/or arm-6
image   = "docker.io/library/debian:bookworm@sha256:..."  # optional
builder = "docker"                # optional: "docker" or "podman"

[[external.<name>.source]]        # zero or more: provenance records only
name    = "mpv"
repo    = "https://github.com/mpv-player/mpv"
ref     = "v0.38.0"
license = "GPL-2.0-or-later"
```

- `image` defaults to the same base image `gosd build-kernel` uses
  (`internal/container.KernelBuildImage`) when omitted, so Docker's layer
  cache stays warm across both commands. Override it if your script needs
  packages that image doesn't have (it's a plain Debian bookworm image —
  `apt-get install` whatever your script needs at the top of the script
  itself, or point `image` at your own image with them preinstalled).
- `builder` is this recipe's own default; the CLI's `--builder` flag always
  wins over it (see the flags table above).
- Every key is validated strictly: an unrecognized key anywhere in the
  file — even nested inside an `[external.<name>]` section or a `[[source]]`
  entry — is an error naming the offending key, the same strictness
  `gosd-kernel.toml` uses (this is a developer-authored build input, not
  `gosd.toml`, so a silent typo should fail loudly).

## Fitting inside the boot partition

`GOSD-BOOT`, the FAT32 partition the kernel, initramfs (which embeds your
app and every bundled external), and bootloader all share, is a fixed
256MiB. A large external (a full-featured video player, e.g.) can eat a
meaningful fraction of that on its own — check the actual built binary's
size (`ls -la ./gosd-externals/<arch>/<name>`, and consider stripping
symbols: `$GOSD_CROSS_COMPILE-strip` on the output before `cp`) and keep an
eye on total image size across a normal `gosd build` run, especially if
you're also compiling a custom kernel (`docs/custom-kernels.md`) — kernel,
initramfs, and every bundled external all have to fit in the same 256MiB
together.

## Supported hosts

`gosd build-external` runs on the same hosts the rest of the CLI
supports — macOS and Linux (amd64/arm64) — with Docker Desktop, colima
(docker-runtime mode; its containerd/nerdctl mode has no docker socket and
is not supported), or Podman installed and its daemon/machine running. All
three share the user's home directory with their VMs, which the build
relies on for its bind mounts (the same requirement `gosd build-kernel`
has — see `docs/custom-kernels.md`). Windows is untested, matching the rest
of the CLI's "best-effort, don't break gratuitously" stance.
