---
# gosd-oyhi
title: 'Externals: build and bundle non-Go binaries into images'
status: todo
type: epic
created_at: 2026-07-13T13:18:59Z
updated_at: 2026-07-13T13:18:59Z
---

A generic mechanism for shipping companion executables ("externals") alongside the user's Go app: `gosd build-external` cross-compiles a binary in Docker/Podman (like `gosd build-kernel`), and `gosd build --with-external` bundles any prebuilt static binary into the image. Driving use case: betamin (separate, unreferenced repo) bundles a static mpv for hardware-decoded video playback, supervised by its app over mpv's JSON IPC. Planned 2026-07-13.

## Locked decisions

- **gosd-init stays single-child**: no multi-process supervision. The app owns externals via os/exec; if the pair wedges, the app exits and the existing backoff supervisor restarts the unit.
- **Fully static binaries only** — the initramfs has no ld.so or library layout. Enforced at build time (ELF PT_INTERP check).
- **GPL carve-out** (mirrors custom-kernels): GoSD never redistributes built externals; developers compile locally from recipe-pinned sources; the builder writes `source.json` provenance (repos/refs/licenses) next to the output.
- **Naming (locked by JP)**: command `gosd build-external`; flag `gosd build --with-external <path>[:<dest>]` (repeatable, dest absolute, default `/bin/<basename>`); recipe `gosd-external.toml` with `[external.<name>]` + `[[external.<name>.source]]`; packages `internal/extbuild` + `internal/extconfig`; output `./gosd-externals/<arch>/<name>` (per-arch, not per-board).
- **No in-repo example** — betamin serves as the unreferenced example repo.
- Docker-required error text mirrors build-kernel's carve-out language; `gosd build` itself never requires Docker.
