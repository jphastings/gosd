---
# gosd-47rm
title: 'gosd build-kernel: Go-orchestrated custom kernel artifact builds'
status: completed
type: epic
priority: normal
created_at: 2026-07-11T07:39:42Z
updated_at: 2026-07-11T19:47:09Z
---

Give developers a first-class, **opt-in** way to build a custom kernel for
their GoSD image — e.g. to compile in a driver GoSD's trimmed kernels cut
(`CONFIG_MEDIA_SUPPORT`, USB DVB, niche sensors) — without touching the fast
default path. Two tiers:

- **No custom drivers** → stock artifacts from the `artifacts/vX.Y.Z` release,
  `gosd build` exactly as today. Never needs Docker.
- **Custom drivers** → declare them in the project repo (`gosd-kernel.toml`)
  and run `gosd build-kernel`: it drives the local Docker/Podman daemon to
  cross-compile the kernel, and emits a flat artifact dir that drops straight
  into `gosd build --artifacts-dir`. Image builds stay fast; only the kernel
  rebuild is slow, and only when inputs change.

We also **dogfood it**: CI's `build-artifacts.yml` kernel jobs call
`gosd build-kernel` to produce the actually-released kernels, so the developer
path and the release path are the same code.

## Locked decisions (JP, 2026-07-10)

1. **Go-native orchestration.** Kernel-build logic moves out of the per-board
   shell scripts into Go behind a declarative per-board `KernelSpec`. The
   kernel `build.sh`/`docker-build.sh` scripts are retired once CI migrates.
   Single source of truth in Go — no permanent shell wrapper.
2. **Full CI dogfood.** `build-artifacts.yml`'s five kernel jobs (pi-zero-2w,
   pi-zero-w, radxa-zero-3e, nanopi-zero2, qemu-virt) switch to
   `gosd build-kernel`. The two U-Boot jobs stay on their current
   `build.sh`/Dockerfile — U-Boot is out of scope for this epic.
3. **Orchestration-only scope.** This epic delivers custom kernels with
   drivers compiled **in** (`=y`; the kernels remain monolithic,
   `CONFIG_MODULES=n`). Loadable `.ko` modules are a separate decision bean
   ([[gosd-2k9p]]), linked but not blocking.
4. **Build-purity carve-out.** The project-wide "no build step may require
   root, Docker, or Linux" decision stands for `gosd build`; `build-kernel` is
   an explicit, opt-in exception that requires a container runtime and says so
   in its errors. CLAUDE.md gets this carve-out recorded when the docs bean
   lands.
5. This epic amends [[gosd-y0x3]]'s "Go developers never compile a kernel"
   contract to "never *have* to compile a kernel; may opt in via
   `gosd build-kernel`".

## Why now

Investigated 2026-07-10 (USB DVB-T question): the shipped kernels are
monolithic and trimmed, so any driver GoSD didn't anticipate is simply
unavailable, and the only workaround is hand-running the board `build.sh`
scripts — undocumented, Pi variants bury the build in a heredoc, and outputs
must be hand-copied into `--artifacts-dir`. The artifact plumbing
(`ResolveArtifacts`' flat-dir filename contract, `package.sh`, the manifest
schema) already supports everything this epic needs; only the orchestration is
missing.

## Summary of Changes (epic complete, 2026-07-11)

All seven children shipped, in dependency order across PRs #64–#67, #69, #71,
#72: [[gosd-fe9w]] (internal/container: docker/podman detection, digest-pinned
build image, streaming runs, typed errors), [[gosd-di6v]] (internal/kernelspec:
declarative per-board specs, fragments/patches embedded in place, drift tests),
[[gosd-x488]] (internal/kernelbuild: generated in-container builds, overlay
semantics, content-addressed cache, source.json provenance), [[gosd-abya]]
(the build-kernel subcommand), [[gosd-hkp7]] (gosd-kernel.toml: strict schema,
firmware flow into the initramfs via gosd build), [[gosd-07fl]] (CI dogfood:
the five kernel release jobs now run gosd build-kernel; shell scripts retired;
proven by a green workflow_dispatch run), [[gosd-1pv0]] (docs/custom-kernels.md
with a PROVEN pi-zero-2w DVB-T worked example, CLAUDE.md carve-out,
COMPATIBILITY row).

Real-world verification did its job: building an actual qemu-virt kernel via
local Docker and booting it to HTTP (~5s) surfaced two field bugs no fake
could catch — [[gosd-0p21]] (/work mount empty: macOS $TMPDIR isn't shared
with Docker Desktop's VM; PR #68) and [[gosd-l4y9]] (macOS storage-pressure
eviction deleted ~/Library/Caches/gosd mid-build; kernel-build state moved to
a durable dir; PR #70). The rp1-cfe media collision was root-caused
(CONFIG_EXPERT defaults MEDIA_PLATFORM_SUPPORT=y → both CSI drivers promoted
to =y under CONFIG_MODULES=n) with the two-line fragment fix documented.

Still open, by design: the [[gosd-2k9p]] loadable-modules decision (not a
child; monolithic-with-compiled-in-drivers is the shipped answer until JP
decides otherwise).

## Child beans

Sequenced: KernelSpec + container runtime first (independent), then the
builder, then the CLI, then config/CI/docs. See each child for its locked
decisions and todos.
