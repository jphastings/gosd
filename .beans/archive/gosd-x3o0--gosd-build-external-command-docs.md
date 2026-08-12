---
# gosd-x3o0
title: gosd build-external command + docs
status: completed
type: feature
priority: normal
created_at: 2026-07-13T13:19:54Z
updated_at: 2026-07-15T04:21:31Z
parent: gosd-oyhi
blocked_by:
    - gosd-sn30
---

`cmd/gosd/buildexternal.go` mirroring buildkernel.go: `--config` (default `./gosd-external.toml`), repeatable `--name` (omit = all recipes) and `--arch` (omit = every arch the recipe declares), `--builder`, `-o` (default `./gosd-externals/`), cache-hit summary lines, closing hint `use it with: gosd build --with-external ./gosd-externals/arm64/<name>`.

## Locked decisions

- Docker-required error text mirrors build-kernel's carve-out language (gosd build itself never requires Docker; build-external does, by design, and says so in --help and errors).
- Docs: `docs/externals.md` — recipe format, static-only contract, GPL carve-out framing, provenance expectations, and generic guidance for big builds (e.g. media players: musl cross toolchain if glibc-static fights back; keep binary + kernel + initramfs within the 256MiB GOSD-BOOT partition). No in-repo example (betamin is the unreferenced example).
- CLAUDE.md: extend the Docker carve-out locked-decision line to cover build-external.

## Todo

- [x] cmd/gosd/buildexternal.go + tests (injected fake runtime/build func, buildkernel_test.go style)
- [x] docs/externals.md
- [x] CLAUDE.md carve-out line + README mention

## Summary of Changes

- Added `gosd build-external` (`cmd/gosd/buildexternal.go`), the front end for
  `internal/extconfig`/`internal/extbuild`: `--config` (default
  `./gosd-external.toml`, required — a recipe is the whole input), repeatable
  `--name` (omit = every recipe) and `--arch` (omit = every arch a recipe
  declares; given, each recipe builds the intersection with its own declared
  arches, erroring by name if a recipe matches none of the requested arches),
  `--builder` (flag wins over each recipe's own `[external.<name>].builder`,
  over auto-detect), `-o`/`--output` (default `./gosd-externals/`). Output is
  `<output>/<arch>/<name>` plus `<output>/<arch>/<name>.source.json`
  (name-prefixed, not extbuild's bare `source.json`, since one arch
  directory can hold several externals' outputs side by side). Per-(name,
  arch) cache-hit/built summary lines, closing with a `gosd build
  --with-external <path>` hint.
- Generalized `internal/container`'s `NotInstalledError`/`DaemonDownError`
  and `Detect` to take the calling command's name (previously hard-coded to
  "gosd build-kernel"), so build-external's own Docker/Podman-missing errors
  name the right command; build-kernel's call site and tests updated to pass
  its name explicitly. No behavior change for build-kernel.
- Tests (`cmd/gosd/buildexternal_test.go`) mirror `buildkernel_test.go`:
  injected fake detect/build funcs, no Docker needed. Covers config
  loading/validation, name/arch selection and their error paths, per-recipe
  builder precedence, output layout (including the source.json collision
  case), cache-hit reporting, and the Docker-missing error naming
  build-external.
- Added `docs/externals.md`: recipe format and command flags, the container
  contract env vars, the fully-static-binary verification contract (with
  musl-toolchain guidance for glibc-static holdouts), the GPL/licensing
  carve-out (GoSD never redistributes; source.json provenance), output
  layout, and a note on fitting within the 256MiB GOSD-BOOT partition
  alongside a kernel and initramfs. Cross-linked from `docs/runtime.md`'s
  `--with-external` section, `README.md`, and `docs/custom-kernels.md`
  (whose own Docker carve-out sentence now also names build-external).
- Extended CLAUDE.md's Docker carve-out locked-decision line to name
  `gosd build-external` alongside `gosd build-kernel`.
