---
# gosd-ig4h
title: 'gosd build --with-external: bundle prebuilt static binaries'
status: todo
type: feature
created_at: 2026-07-13T13:19:31Z
updated_at: 2026-07-13T13:19:31Z
parent: gosd-oyhi
---

The smallest, highest-leverage piece of the externals epic — lands first and unblocks betamin development with any binary. Repeatable `--with-external <path>[:<dest>]` flag on `gosd build` (cmd/gosd/build.go); files land in the initramfs at mode 0755.

## Locked decisions

- Dest parsing: split on the **last** colon only when the suffix starts with `/`; dest must be absolute; default `/bin/<basename>`.
- Validation: dest must not collide with `/init`, `/app`, `/etc/gosd/*`, `/lib/firmware/*`, or another --with-external.
- Pipeline: `pipeline.Options.ExtraExecutables map[string]io.Reader` (dest-keyed) — the exact ExtraFirmware pattern (internal/pipeline/pipeline.go:56-66, 97-99, 127-141).
- Pre-flight `debug/elf` checks in cmd/gosd: ELF machine/class must match the board's Arch (arm64 vs arm/GOARM=6 — an arm64 binary with --board pi-zero-w is a per-board hard error); PT_INTERP must be absent (actionable static-only error). Errors follow the actionable-error convention.

## Todo

- [ ] Flag parsing + validation in cmd/gosd/build.go
- [ ] pipeline.Options.ExtraExecutables + initramfs wiring
- [ ] ELF pre-flight (arch match, no PT_INTERP)
- [ ] Integration test (fake artifacts, network tripwire, initramfs readback asserting dest+mode across boards; arch-mismatch + dynamic-ELF failure cases; CGO_ENABLED=0 cross-compiled Go test binary as the static-ELF fixture)
- [ ] Docs: README + runtime docs mention
