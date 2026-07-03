---
# gosd-56xt
title: Scaffold gosd CLI and cross-compilation of the user app
status: completed
type: task
priority: normal
created_at: 2026-07-02T20:53:02Z
updated_at: 2026-07-03T06:05:06Z
parent: gosd-vi0n
---

Create the repo skeleton and the `gosd build` command up to producing a static arm64 binary of the user app.

Layout (locked): `go mod init github.com/jphastings/gosd` (adjust module path only if a git remote already says otherwise). Directories: `cmd/gosd` (CLI, use github.com/spf13/cobra), `cmd/gosd-init` (stub main for now), `internal/build`, `internal/image`, `internal/initramfs`, `internal/boards`, `examples/hello`.

`gosd build <path-to-main-package> --board=<pi-zero-2w|radxa-zero-3e> -o <out.img> [--hostname=NAME] [--wifi-ssid=S --wifi-pass=P]` — board flag required; wifi flags are the v0.1 baked-credentials path.

Cross-compile by invoking the host Go toolchain (`go build`) as a subprocess with env `CGO_ENABLED=0 GOOS=linux GOARCH=arm64`, output to a temp dir. Also compile `cmd/gosd-init` the same way. Fail with a clear error if the package is not a main package or the build fails, surfacing go build stderr verbatim.

- [x] Repo skeleton + go.mod
- [x] cobra CLI with build subcommand and flag validation
- [x] internal/build: cross-compile helper with unit test (compiles a testdata main package, asserts output is ELF arm64 statically linked — check ELF header bytes, no test on running it)
- [x] Stub the rest of the build pipeline behind interfaces so image/initramfs tasks slot in

## Acceptance
`go run ./cmd/gosd build ./examples/hello --board=pi-zero-2w -o /tmp/x.img` gets as far as "image assembly not implemented" with a compiled app binary in the temp dir. `go test ./...` passes on macOS and Linux.

## Decisions ratified 2026-07-02 (see CLAUDE.md)
Module path confirmed: github.com/jphastings/gosd. LICENSE (MIT) and CLAUDE.md already exist in the repo — do not recreate them. Flag semantics update: `--board` is optional and repeatable; with no `--board`, build ALL registered boards, output named `<appname>-<board>.img` (`-o` sets a name template or directory in the multi-board case — keep it simple: `-o` names the file only when exactly one board is selected, otherwise it names the output directory). Default hostname when --hostname is absent: sanitized basename of the main package (lowercase, [a-z0-9-] only).

## Summary of Changes

Scaffolded the gosd repo end to end:

- `go.mod` (`github.com/jphastings/gosd`, no existing git remote to override it).
- `cmd/gosd`: cobra-based CLI. `gosd build <path-to-main-package>` supports
  `--board` (optional, repeatable — defaults to all registered boards per the
  ratified decision), `-o/--output` (names the output file when exactly one
  board is selected, otherwise names the output directory; per-board files are
  always `<appname>-<board>.img`), `--hostname` (defaults to the sanitized
  main-package basename), `--wifi-ssid`/`--wifi-pass`. Unknown `--board`
  values are rejected with the list of valid IDs.
- `internal/boards`: registry of `pi-zero-2w` and `radxa-zero-3e`.
- `internal/naming`: shared sanitizer (`[a-z0-9-]`, lowercase) used for both
  the default hostname and output filenames; unit tested.
- `internal/build`: `CrossCompile(pkgPath, outputPath)` shells out to the host
  `go build` with `CGO_ENABLED=0 GOOS=linux GOARCH=arm64`, pre-checking via
  `go list -f {{.Name}}` that the target is `package main`, and surfacing
  `go build`'s stderr verbatim on failure. Unit tests build a real
  `testdata/hello` main package and assert the result is a 64-bit ARM64 ELF
  binary with no `PT_INTERP` segment (statically linked); a second test
  asserts a non-main `testdata/notmain` package is rejected.
- `internal/image` and `internal/initramfs`: interfaces (`Assembler.Assemble`,
  `Builder.Build`) other beans will implement, each with a `NotImplemented`
  stub returning a clear "image/initramfs assembly not implemented" error.
  `image.Spec` carries the cross-compiled app/init binary paths, board,
  hostname, WiFi credentials, output path, and an `initramfs.Builder` for the
  real assembler to call.
- `cmd/gosd-init` and `examples/hello`: minimal placeholder `main` packages
  for other beans to flesh out.
- `gosd build` orchestration: resolves boards and output paths, cross-compiles
  the app and `gosd-init` into a temp dir (left in place, not cleaned up), then
  calls `image.NotImplemented.Assemble` per board, which fails with "image
  assembly not implemented" — matching the bean's acceptance criterion.

Verified with:
`go run ./cmd/gosd build ./examples/hello --board=pi-zero-2w -o /tmp/x.img`
→ `gosd: building hello for pi-zero-2w failed: image assembly not implemented`
(exit 1), with both `hello` and `gosd-init` present in the temp build dir as
statically linked ARM64 ELF binaries (confirmed via `file`). Also verified the
no-`--board` (build-all) path and the unknown-board rejection path.

`go build ./...`, `go vet ./...`, `gofmt -l .` (empty), and `go test ./...`
all pass.

### Deviations / notes for follow-on beans

- `cmd/gosd-init` is cross-compiled via its module import path
  (`github.com/jphastings/gosd/cmd/gosd-init`), not a relative path, so it
  resolves independent of the caller's working directory. This only works
  when `gosd` is run from within a checkout of its own module; packaging/
  distributing the `gosd` binary for use against other repos will need to
  address how `gosd-init`'s source is located (embedding, prebuilt binaries,
  or similar) — out of scope here, flagging for whichever bean owns
  distribution.
- `internal/naming` wasn't named in the bean's layout list but was added as a
  small shared package since both the hostname default and output filenames
  need the same sanitization rule; it's a private implementation detail, not
  part of the public API surface.
- The image-assembly bean should implement `image.Assembler` (see
  `internal/image/image.go`) and can call `spec.Initramfs.Build(ctx, ...)`
  (see `internal/initramfs/initramfs.go`) to get the initramfs archive before
  finishing partitioning/assembly. Both currently only have `NotImplemented`
  stubs.
