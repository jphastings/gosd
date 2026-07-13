---
# gosd-x3o0
title: gosd build-external command + docs
status: todo
type: feature
priority: normal
created_at: 2026-07-13T13:19:54Z
updated_at: 2026-07-13T13:26:11Z
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

- [ ] cmd/gosd/buildexternal.go + tests (injected fake runtime/build func, buildkernel_test.go style)
- [ ] docs/externals.md
- [ ] CLAUDE.md carve-out line + README mention
