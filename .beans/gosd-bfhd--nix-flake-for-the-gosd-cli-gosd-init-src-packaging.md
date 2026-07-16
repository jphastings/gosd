---
# gosd-bfhd
title: Nix flake for the gosd CLI (+ GOSD_INIT_SRC packaging hook)
status: completed
type: task
priority: normal
created_at: 2026-07-16T22:46:59Z
updated_at: 2026-07-16T22:57:01Z
---

Package the gosd CLI as a nix flake at the repo root, so CI systems with nix (notably Tangled spindle runners) can use gosd via `nix run github:jphastings/gosd` / a flake input without any release infrastructure.

## Locked decisions

- **flake.nix at repo root**, `buildGoModule`, `subPackages = ["cmd/gosd"]`, `CGO_ENABLED=0`, `doCheck = false` (the test suite runs in regular CI; the nix sandbox forbids network access and duplicating the suite buys nothing).
- **vendorHash freshness is CI-enforced**: a `nix build` job in ci.yml goes red with nix's "hash mismatch / got: sha256-…" error whenever go.mod/go.sum change without the flake being updated; flake.nix carries a comment pointing at that workflow.
- **GOSD_INIT_SRC env var** (new, tiny): `--gosd-init-src`'s default comes from the environment. Rationale: a nix-built gosd has no VCS build info (`Main.Version` = `(devel)`) and `-trimpath` defeats the runtime.Caller fallback, so both detection rungs in internal/build/gosdinit.go fail for downstream apps — exactly the packaged-binary use case. Explicit flag still beats env. `GOSD_*` is already gosd's reserved env prefix.
- **The package ships its own gosd-init source**: postInstall copies the configured source tree (including the vendor dir buildGoModule already wired) to `$out/share/gosd-src`, and the binary is wrapped with `--set-default GOSD_INIT_SRC $out/share/gosd-src/cmd/gosd-init` (the flag/env expects the *main package* dir, not the module root). Consequence: gosd-init compiles **offline** from a nix-installed gosd — no module-proxy fetch, no GitHub dependency at image-build time.
- flake.lock committed; nixpkgs input pinned to nixos-unstable (needs go >= go.mod's directive).
- Not in scope: docker image publishing (dockerTools can wrap this package later if wanted), nix-based dev shells beyond a minimal one, pushing gosd to nixpkgs proper.

## Todo

- [x] GOSD_INIT_SRC env default for --gosd-init-src + behavioral test + flag help text
- [x] flake.nix + flake.lock + .gitignore result symlink
- [x] Real vendorHash computed and full `nix build` verified (container on the remote build box), including an offline `go -C $out/share/gosd-src build ./cmd/gosd-init` proof
- [x] CI nix job in ci.yml
- [x] README note (install/run via nix)

## Summary of Changes

Branch `bean/gosd-bfhd-nix-flake`.

- `cmd/gosd/build.go`: `--gosd-init-src` defaults to `$GOSD_INIT_SRC` (flag
  still wins); help text names the env var as the package-manager hook. Two
  behavioral tests in build_test.go.
- `flake.nix` + `flake.lock` (nixpkgs nixos-unstable @ 2026-07-15):
  `buildGoModule` of cmd/gosd, real `vendorHash`
  (sha256-1pPplh3xJcqyuQesLgmh/…), `allowGoReference = true` (gosd invokes
  go at run time by design), postInstall copies the vendored source tree to
  `$out/share/gosd-src` and wraps the binary with
  `GOSD_INIT_SRC=$out/share/gosd-src/cmd/gosd-init` + go appended to PATH
  as a fallback.
- `.github/workflows/ci.yml`: `nix-build` job (checkout + pinned
  cachix/install-nix-action v31.11.0 + `nix build` + `gosd --help` smoke) —
  this is the vendorHash-drift gate the bean requires.
- README: nix run one-liner next to `go install`.
- `.gitignore`: `/result`.

**Verified for real** (nixos/nix container on the remote build box,
linux/arm64): full `nix build` succeeds; then, with `GOPROXY=off` (all
network fetches hard-disabled), the wrapped gosd built
`examples/hello --board pi-zero-2w --artifacts-dir <fake>` end to end —
app cross-compile, gosd-init compile from the bundled vendored source via
the wrapper's GOSD_INIT_SRC, image assembly — producing a 285 MB image.
Offline operation confirmed, not assumed.

**Two build-time findings encoded in flake.nix comments:**
- buildGoModule's default no-go-references check fails this package
  (wrapper puts go on PATH) — `allowGoReference = true` is the intended
  switch, not a workaround.
- The flag/env must point at `cmd/gosd-init` (the main package dir), not
  the module root — `crossCompileInDir(overrideDir, ".")` builds "." in
  that directory.
