---
# gosd-sn30
title: 'internal/extbuild + internal/extconfig: containerized external builds'
status: completed
type: feature
priority: normal
created_at: 2026-07-13T13:19:31Z
updated_at: 2026-07-15T02:08:47Z
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

- [x] internal/extconfig parser + tests
- [x] internal/extbuild builder + cache + provenance
- [x] Fake-runner unit tests (kernelbuild_test.go style, no daemon in CI)

## Summary of Changes

- Added `internal/staticelf` (new package): extracted the ELF class/machine
  + PT_INTERP static-linkage check that previously lived only in
  `cmd/gosd/external.go` (bean gosd-ig4h), keyed on `boards.Arch`. It
  exposes `Verify`, `Expectations`, `GOARMSuffix`, and typed errors
  (`NotELFError`, `MismatchError`, `DynamicallyLinkedError`) so callers can
  compose their own audience-specific wording from structured fields.
  Refactored `cmd/gosd/external.go`'s `validateStaticELF` to call
  `staticelf.Verify` and reconstruct its exact prior error text from the
  typed errors - its existing tests (including the withexternal integration
  tests) pass unmodified.
- Added `internal/extconfig`: a strict `gosd-external.toml` parser mirroring
  `internal/kernelconfig`'s idiom (unknown keys anywhere are errors, decoded
  via `toml.Primitive` + a shared `MetaData.Undecoded()` check). Parses
  `[external.<name>]` (script, arch list, optional image/builder) and
  `[[external.<name>.source]]` provenance entries (name/repo/ref/license,
  all required). Arch tokens validate against a new `boards.KnownArches`
  vocabulary (`arm64`, `arm-6`) added to `internal/boards`, since the live
  board registry is only populated by `cmd/gosd`'s `init()` and isn't
  available to a standalone parser. `External.ScriptPath`/`ReadScript`
  resolve a relative script path against the TOML's own directory.
- Added `internal/extbuild`: a sibling builder to `internal/kernelbuild`
  (not a parameterization of it), driving one `Spec` (name, script bytes,
  target arch, provenance sources) through `internal/container` per Build
  call. Mounts `/work` (developer script + a small generated `wrapper.sh`
  entrypoint) read-only and `/out` read-write; sets `GOSD_ARCH` (the
  recipe's own token, e.g. `arm-6`), `GOSD_CROSS_COMPILE`, and
  `GOSD_OUTPUT=/out/<name>`. The cache key hashes script bytes + image
  digest + arch + output name (sha256, JSON-marshaled) into a durable
  per-OS state dir (`~/Library/Application Support/gosd/ext-build` etc.,
  mirroring kernelbuild's eviction-safe location); a cache hit skips the
  container entirely. After a run, `verifyOutput` opens `/out/<name>` and
  calls `staticelf.Verify` before the result is trusted into the cache -
  a wrong-arch or dynamically linked output fails the build outright.
  `writeSourceJSON` writes every `Spec.Sources` entry to `source.json`
  (always written, even as an empty array) for GPL-carve-out provenance.
- No deviations from the bean's locked decisions. This bean intentionally
  stops short of wiring extconfig/extbuild together or into a CLI command -
  that glue is bean gosd-x3o0's job.
