---
# gosd-sn30
title: 'internal/extbuild + internal/extconfig: containerized external builds'
status: todo
type: feature
created_at: 2026-07-13T13:19:31Z
updated_at: 2026-07-13T13:19:31Z
parent: gosd-oyhi
---

The build machinery behind `gosd build-external`: `internal/extconfig` parses `gosd-external.toml` strictly (mirror internal/kernelconfig: unknown keys are errors; arch values validated against the boards arch vocabulary {arm64, arm-6}; script path resolved against the TOML's dir). `internal/extbuild` is a **sibling** of internal/kernelbuild (which is kbuild-shaped throughout — do not parameterize it), reusing `internal/container.RunSpec`.

## Locked decisions

- Recipe shape: `[external.<name>]` with `script`, `arch = ["arm64"]`, optional `image`/`builder`; `[[external.<name>.source]]` provenance entries (name, repo, ref, license) — provenance-recording only; the script does the actual pinned cloning.
- Container contract: `/work` RO (developer script + generated thin wrapper), `/out` RW; env `GOSD_ARCH`, `GOSD_CROSS_COMPILE`, `GOSD_OUTPUT=/out/<name>`.
- Default image shared with kernel builds (apt-layer cache warmth — JP's explicit ask).
- Content-addressed durable cache like kernelbuild (key: script bytes, image digest, arch, output name; same state-dir pattern — bind mounts stage under the user's home, never /var/folders or ~/Library/Caches).
- Post-run verification: output exists, ELF machine matches arch, no PT_INTERP; write source.json from the recipe's sources.

## Todo

- [ ] internal/extconfig parser + tests
- [ ] internal/extbuild builder + cache + provenance
- [ ] Fake-runner unit tests (kernelbuild_test.go style, no daemon in CI)
