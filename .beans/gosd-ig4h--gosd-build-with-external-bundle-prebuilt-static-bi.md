---
# gosd-ig4h
title: 'gosd build --with-external: bundle prebuilt static binaries'
status: completed
type: feature
priority: normal
created_at: 2026-07-13T13:19:31Z
updated_at: 2026-07-14T09:52:56Z
parent: gosd-oyhi
---

The smallest, highest-leverage piece of the externals epic — lands first and unblocks betamin development with any binary. Repeatable `--with-external <path>[:<dest>]` flag on `gosd build` (cmd/gosd/build.go); files land in the initramfs at mode 0755.

## Locked decisions

- Dest parsing: split on the **last** colon only when the suffix starts with `/`; dest must be absolute; default `/bin/<basename>`.
- Validation: dest must not collide with `/init`, `/app`, `/etc/gosd/*`, `/lib/firmware/*`, or another --with-external.
- Pipeline: `pipeline.Options.ExtraExecutables map[string]io.Reader` (dest-keyed) — the exact ExtraFirmware pattern (internal/pipeline/pipeline.go:56-66, 97-99, 127-141).
- Pre-flight `debug/elf` checks in cmd/gosd: ELF machine/class must match the board's Arch (arm64 vs arm/GOARM=6 — an arm64 binary with --board pi-zero-w is a per-board hard error); PT_INTERP must be absent (actionable static-only error). Errors follow the actionable-error convention.

## Todo

- [x] Flag parsing + validation in cmd/gosd/build.go
- [x] pipeline.Options.ExtraExecutables + initramfs wiring
- [x] ELF pre-flight (arch match, no PT_INTERP)
- [x] Integration test (fake artifacts, network tripwire, initramfs readback asserting dest+mode across boards; arch-mismatch + dynamic-ELF failure cases; CGO_ENABLED=0 cross-compiled Go test binary as the static-ELF fixture)
- [x] Docs: README + runtime docs mention

## Summary of Changes

Added a repeatable `gosd build --with-external <path>[:<dest>]` flag (cmd/gosd/external.go) that bundles a prebuilt static executable into the image's initramfs. Dest parsing splits on the last colon only when the suffix starts with "/", defaults to `/bin/<basename>`, and is validated up front against collisions with `/init`, `/app`, `/etc/gosd/*`, `/lib/firmware/*`, and duplicate --with-external dests. `pipeline.Options` gained `ExtraExecutables map[string]io.Reader` (dest-keyed), written into the initramfs at mode 0755, mirroring ExtraFirmware's shape and reader-closing convention exactly. Per-board ELF pre-flight (stdlib debug/elf, no new deps) opens each external once per selected board (mirroring how ExtraFirmware readers are opened per board), checks its ELF class/machine against that board's Arch (arm64 vs arm/GOARM=6), and rejects any ELF with a PT_INTERP program header as dynamically linked, all with actionable errors. Docs added to README.md, docs/runtime.md, and COMPATIBILITY.md. Tests: unit tests for dest-splitting/validation, plus a fixture-driven integration test suite (network-tripwire transport, real CGO_ENABLED=0 cross-compiled Go binaries as static-ELF fixtures for arm64/armv6, a hand-crafted PT_INTERP ELF for the dynamic-link rejection case) covering default/explicit dest, mode 0755, multi-board sharing one arch, arch-mismatch, dynamic-linking, non-ELF, and reserved-dest-collision failures.
