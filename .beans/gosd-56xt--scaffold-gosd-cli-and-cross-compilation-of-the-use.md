---
# gosd-56xt
title: Scaffold gosd CLI and cross-compilation of the user app
status: todo
type: task
priority: normal
created_at: 2026-07-02T20:53:02Z
updated_at: 2026-07-02T21:17:59Z
parent: gosd-vi0n
---

Create the repo skeleton and the `gosd build` command up to producing a static arm64 binary of the user app.

Layout (locked): `go mod init github.com/jphastings/gosd` (adjust module path only if a git remote already says otherwise). Directories: `cmd/gosd` (CLI, use github.com/spf13/cobra), `cmd/gosd-init` (stub main for now), `internal/build`, `internal/image`, `internal/initramfs`, `internal/boards`, `examples/hello`.

`gosd build <path-to-main-package> --board=<pi-zero-2w|radxa-zero-3e> -o <out.img> [--hostname=NAME] [--wifi-ssid=S --wifi-pass=P]` — board flag required; wifi flags are the v0.1 baked-credentials path.

Cross-compile by invoking the host Go toolchain (`go build`) as a subprocess with env `CGO_ENABLED=0 GOOS=linux GOARCH=arm64`, output to a temp dir. Also compile `cmd/gosd-init` the same way. Fail with a clear error if the package is not a main package or the build fails, surfacing go build stderr verbatim.

- [ ] Repo skeleton + go.mod
- [ ] cobra CLI with build subcommand and flag validation
- [ ] internal/build: cross-compile helper with unit test (compiles a testdata main package, asserts output is ELF arm64 statically linked — check ELF header bytes, no test on running it)
- [ ] Stub the rest of the build pipeline behind interfaces so image/initramfs tasks slot in

## Acceptance
`go run ./cmd/gosd build ./examples/hello --board=pi-zero-2w -o /tmp/x.img` gets as far as "image assembly not implemented" with a compiled app binary in the temp dir. `go test ./...` passes on macOS and Linux.

## Decisions ratified 2026-07-02 (see CLAUDE.md)
Module path confirmed: github.com/jphastings/gosd. LICENSE (MIT) and CLAUDE.md already exist in the repo — do not recreate them. Flag semantics update: `--board` is optional and repeatable; with no `--board`, build ALL registered boards, output named `<appname>-<board>.img` (`-o` sets a name template or directory in the multi-board case — keep it simple: `-o` names the file only when exactly one board is selected, otherwise it names the output directory). Default hostname when --hostname is absent: sanitized basename of the main package (lowercase, [a-z0-9-] only).
