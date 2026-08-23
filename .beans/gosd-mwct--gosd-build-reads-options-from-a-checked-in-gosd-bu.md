---
# gosd-mwct
title: gosd build reads options from a checked-in gosd-build.toml
status: in-progress
type: feature
created_at: 2026-08-23T19:58:13Z
updated_at: 2026-08-23T19:58:13Z
---

`gosd build` has ~22 flags; developers who need a specific combination carry a long command line in a Makefile. A checked-in `gosd-build.toml` lets a repo declare its canonical build, with CLI flags still winning.

## Locked decisions (JP, 2026-08-23)

- **Every build flag is file-settable; CLI flags override, per key** (`IsSet && !Changed`; CLI arrays replace file arrays wholesale). Exception: `gosd-init-src` is flag > `$GOSD_INIT_SRC` > file — the env var is a per-machine installation pin (nix wrappers) and must beat a file that travels between machines.
- **Structural flag↔key mapping, no hand-maintained map**: a flag `--<section>-<rest>` where `<section>` is a declared table maps to `[section] rest`; every other flag is a top-level key of its own name. Sections: `[app]`, `[boot]`, `[data]`, `[kernel]`, `[publish]`. A reflection-driven parity test pins both directions.
- **Three shipped flags get a clean rename** (no deprecation aliases, house 0.x style) so real groups fit the rule: `--support-url` → `--app-support-url`, `--config-dir` → `--boot-config-dir` (build AND run), `--catalog` → `--publish-catalog`.
- **`[app] main`** (the one key with no flag — it supplies the positional package path) makes a bare `gosd build` work in a checked-out repo. A positional still wins; no file + no positional stays an error.
- **Discovery: `gosd-build.toml` stat'd in the cwd only** (exactly like `gosd-kernel.toml`; no walk-up). Missing → zero config. New CLI-only `--build-config <path>` escape hatch on build and run; explicit-but-missing errors.
- **Relative paths in the file resolve against the file's own directory** (the kernel/external toml convention): `output`, `boot.config-dir`, `artifacts-dir`, `gosd-init-src`, `kernel.config`, `app.main` (filesystem-relative forms only), and the path half of `with-external`. `placeholder` paths are on-image and never rebased.
- **`gosd run` reads the same file** for the flags it already mirrors (`app.main`, `boot.size`, `boot.config-dir`, `data.size`, `data.flush`, `kernel.param`, `label-prefix`, `ingress`, `artifacts-dir`, `gosd-init-src`); build-only keys are silently ignored under run — matching run's existing silent-divergence design (no `--data-filesystem` by design).
- **Strict TOML** via BurntSushi, mirroring internal/kernelconfig ([[gosd-hkp7]]): any unknown key anywhere errors naming the dotted key. No semantic validation in Parse — merged values flow through the existing cmd/gosd parse helpers.
- **File-set `label-prefix` counts as explicit** at the two resolveLabels call sites (empty errors, like `--label-prefix=""`).
- **Flat per board in v1** — no per-board override tables; run-only flags (`--port`, `--memory`, …) not file-settable in v1. Both additive later.
- The name `gosd.toml` is burned ([[gosd-rw6n]]); `gosd-build.toml` fits the gosd-<verb>.toml surface.

## Todo

- [ ] Rename sweep: `--support-url`→`--app-support-url`, `--config-dir`→`--boot-config-dir` (build+run), `--catalog`→`--publish-catalog`, incl. help texts, pairing error, tests, README, docs
- [ ] `internal/buildconfig`: strict Parse, IsSet (dotted), reflection-derived Keys, ResolvePath + unit tests
- [ ] `cmd/gosd/buildconfigfile.go`: loadBuildConfig, fileKey tables, applyFileValues, resolveMainOperand + unit tests incl. structural parity test
- [ ] Wire runBuild/runRun: MaximumNArgs(1), prologue, label-prefix explicitness, `--build-config` flags, run's gosd-init-src env default
- [ ] Integration tests: file-only bare build, flag-override, --build-config elsewhere, bare run w/ ignored build-only keys, no-file-no-arg errors
- [ ] docs/build-config.md + README + changeset (`gosd: major`)
- [ ] Quality gates (go test/vet/gofmt/golangci-lint darwin+linux)
