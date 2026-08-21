---
# gosd-oyhi
title: 'Externals: build and bundle non-Go binaries into images'
status: completed
type: epic
priority: normal
created_at: 2026-07-13T13:18:59Z
updated_at: 2026-08-21T01:41:43Z
---

A generic mechanism for shipping companion executables ("externals") alongside the user's Go app: `gosd build-external` cross-compiles a binary in Docker/Podman (like `gosd build-kernel`), and `gosd build --with-external` bundles any prebuilt static binary into the image. Driving use case: betamin (separate, unreferenced repo) bundles a static mpv for hardware-decoded video playback, supervised by its app over mpv's JSON IPC. Planned 2026-07-13.

## Locked decisions

- **gosd-init stays single-child**: no multi-process supervision. The app owns externals via os/exec; if the pair wedges, the app exits and the existing backoff supervisor restarts the unit.
- **Fully static binaries only** — the initramfs has no ld.so or library layout. Enforced at build time (ELF PT_INTERP check).
- **GPL carve-out** (mirrors custom-kernels): GoSD never redistributes built externals; developers compile locally from recipe-pinned sources; the builder writes `source.json` provenance (repos/refs/licenses) next to the output.
- **Naming (locked by JP)**: command `gosd build-external`; flag `gosd build --with-external <path>[:<dest>]` (repeatable, dest absolute, default `/bin/<basename>`); recipe `gosd-external.toml` with `[external.<name>]` + `[[external.<name>.source]]`; packages `internal/extbuild` + `internal/extconfig`; output `./gosd-externals/<arch>/<name>` (per-arch, not per-board).
- **No in-repo example** — betamin serves as the unreferenced example repo.
- Docker-required error text mirrors build-kernel's carve-out language; `gosd build` itself never requires Docker.


---

**Amendment recorded (gosd-66ax, 2026-08-07):** the cloudflared-ingress
epic [[gosd-virc]] carves a narrow exception into the single-child decision
above: gosd-SHIPPED system services (first: cloudflared) may be
gosd-init-supervised — wired as `guard.Go("cloudflared", ...)` inside
cmd/gosd-init/main.go's StartNetworking, supervised through the PID-1
reaper's Wait exactly like `/app` (never through boot.Supervisor). USER
externals remain app-owned via os/exec, unchanged: the "gosd-init stays
single-child" bullet above still governs `--with-external` companion
binaries. The carve-out text landed in boot/reaper.go's stash comment and
docs/runtime.md's "Your app owns it at runtime" bullet, both in
[[gosd-66ax]].

## Summary of Changes

Companion non-Go binaries can now be built and bundled. gosd-sn30 wrote
`internal/extconfig` (strict `gosd-external.toml` parsing —
`[external.<name>]` plus `[[external.<name>.source]]`, unknown keys are an
error, mirroring the kernel-recipe idiom) and `internal/extbuild` (the
containerized cross-compile). gosd-x3o0 exposed it as `gosd build-external`,
which requires Docker or Podman and says so in its own `--help` and errors,
per the carve-out — `gosd build` itself still never needs a container.
gosd-ig4h added `gosd build --with-external <path>[:<dest>]`, repeatable,
dest absolute, defaulting to `/bin/<basename>`.

The two contracts that make this safe are enforced, not documented: static
linking is checked by an ELF `PT_INTERP` probe in `internal/staticelf`,
shared by both entry points so a dynamically-linked binary is refused before
it can reach an initramfs that has no `ld.so`; and provenance is written as
`source.json` beside each output (per-name on the shared per-arch output
dir), which is what keeps GoSD out of the business of redistributing built
externals. `docs/externals.md` documents both. There is deliberately no
in-repo example recipe.

The single-child supervision decision above survived with one carve-out,
recorded here by gosd-66ax: gosd-SHIPPED system services (cloudflared, and
later the tailscale-funnel shim) may be gosd-init-supervised. USER externals
remain app-owned via `os/exec`, unchanged.
