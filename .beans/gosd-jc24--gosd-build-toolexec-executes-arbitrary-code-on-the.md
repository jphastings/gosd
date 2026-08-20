---
# gosd-jc24
title: gosd build -- -toolexec=... executes arbitrary code on the build host (argument injection)
status: completed
type: bug
created_at: 2026-08-12T04:13:13Z
updated_at: 2026-08-12T04:13:13Z
---

**Severity: High.** Arbitrary code execution on a developer's or CI machine,
with a one-line fix. Precondition: the attacker influences gosd's argv.

## Verified — code actually executed

```
$ gosd build --board qemu-virt -- "-toolexec=/tmp/tx.sh"
gosd build: boot partition label: tx-sh-boot
gosd build: qemu-virt boot volume: 68MiB / 256MiB used (26.7%)
$ ls /tmp/PWNED_MARKER
-rw-r--r--  1 jp  wheel  0 ... /tmp/PWNED_MARKER
```

`tx.sh` was `#!/bin/sh; touch /tmp/PWNED_MARKER; exec "$@"`. It ran. Note the
derived label `tx-sh-boot` — the injected flag was consumed as the package
path all the way through the pipeline.

Bare `gosd build -toolexec=...` is blocked (cobra parses it as an unknown
flag), but cobra honours `--` as a flag terminator, after which the value
becomes `args[0]`.

## Mechanism

`cmd/gosd/build.go:181-182` takes `pkgPath := args[0]` under
`cobra.ExactArgs(1)` with no validation. It reaches:

- `internal/build/build.go:39` — `args = append(args, pkgPath)`, run via
  `exec.Command("go", args...)` at `:41`
- `internal/build/build.go:70` — `exec.Command("go", "list", "-f", "{{.Name}}", pkgPath)`

Neither inserts Go's `--` terminator nor rejects a leading `-`, so `go`
parses the value as a build flag. `-toolexec` runs an arbitrary program in
place of `compile`/`asm`/`link` for every build step. `requireMainPackage`
is not a gate: `go list` with only flags falls back to the CWD package, so
a main package in the working directory satisfies it and the build proceeds.

## Attack

Any wrapper that forwards a not-fully-trusted string into `gosd build`:
a CI job templating a branch/PR-derived value, a Makefile `gosd build
$(PKG)`, a build script taking a package argument. The attacker supplies
`-toolexec=/path/to/payload` and gets code execution on the build host
before the app is even compiled — which, for a tool whose output is
flashed to hardware, means the attacker also controls every image that
build produces.

## Related, same file — `GOFLAGS` is inherited unfiltered

`internal/build/build.go:57-67` (`archEnv`) does `append(os.Environ(), ...)`,
so the `go` subprocess inherits the entire ambient environment. A
`GOFLAGS=-toolexec=...` set by a `.envrc` (direnv, on entering a hostile
cloned repo), a poisoned shell profile, or an inherited CI variable reaches
the compiler with no filtering — the same RCE class through a vector that
does not need argv control at all.

## Fix

Both are small and independent:

- Insert Go's flag terminator immediately before the package operand in
  every constructed argv: `args = append(args, "--", pkgPath)` in
  `CrossCompile` (build.go:39) and in `requireMainPackage` (build.go:70).
  Belt and braces, reject a `pkgPath` starting with `-` in
  `cmd/gosd/build.go` with an actionable error.
- Strip `GOFLAGS` (and audit `GOPROXY`/`GOPRIVATE`) in `archEnv` rather than
  inheriting whatever is set.

## Todos

- [x] `--` terminator before `pkgPath` in `CrossCompile` and `requireMainPackage`
- [x] Reject a leading-dash `pkgPath` at the CLI boundary with a clear error
- [x] Strip `GOFLAGS` in `archEnv`
- [x] Check `buildexternal.go` / `buildkernel.go` for the same trailing-operand shape
- [x] Test: `gosd build -- -toolexec=<marker script>` fails, and the marker is not created

## Reproduced before fixing

Both vectors were reproduced against a binary built from `main`, with the
marker written into a scratch directory rather than `/tmp/PWNED_MARKER`:

```
$ gosd build --board qemu-virt -- "-toolexec=$SCRATCH/tx.sh"
gosd build: boot partition label: tx-sh-boot
gosd build: qemu-virt boot volume: 65MiB / 256MiB used (25.4%)
gosd build: qemu-virt inject manifest: tx-sh-qemu-virt.inject.json
$ ls $SCRATCH/PWNED_MARKER_JC24
-rw-r--r--  1 jp  wheel  0 ... PWNED_MARKER_JC24

$ GOFLAGS="-toolexec=$SCRATCH/tx.sh" gosd build --board qemu-virt .
gosd build: boot partition label: app-boot
$ ls $SCRATCH/PWNED_MARKER_JC24
-rw-r--r--  1 jp  wheel  0 ... PWNED_MARKER_JC24
```

Re-run against the fix, all three invocations (`build` argv, `run` argv,
`GOFLAGS`) leave no marker; the two argv ones exit 1 at the CLI boundary
before any subprocess starts, and the `GOFLAGS` one compiles and assembles
an image normally with the payload never invoked.

The new tests were confirmed to fail against pre-fix code by reverting the
three source files and re-running them: both `internal/build` tests fail on
"the -toolexec payload ran".

## Scope check

`gosd run` shared the hole exactly — same `args[0]`, same
`build.CrossCompile` — and is fixed by the same two changes.
`gosd build-kernel` and `gosd build-external` take no positional arguments
at all (flags only), so they have no trailing-operand shape to exploit.
`--gosd-init-src` is a flag value, not an operand, and reaches the toolchain
as `go -C <dir>` where it cannot be read as a flag; `internal/build`'s
`go list -m` and `go mod download` operands are built from compile-time
constants and gosd's own build version, not from user input.

## Summary of Changes

Three changes, each independently sufficient against a different part of the
attack:

- `cmd/gosd`: `validatePkgPath` rejects a positional argument that isn't
  recognisably a Go package path, from both `runBuild` and `runRun`, as the
  first thing either does. It is an allow-list on the shape of a package
  path — a relative path, an absolute path, or an import path — rather than
  a blacklist of flag names, because `-toolexec` is only today's worst flag
  and `-ldflags`, `-exec`, `-overlay` and whatever Go adds next would each
  have to be remembered. The error names the rejected argument, says why
  (an argument starting with `-` is read as a build flag, and flags like
  `-toolexec` run arbitrary programs), and gives valid examples.
- `internal/build`: `CrossCompile` and `requireMainPackage` both pass Go's
  `--` terminator immediately before `pkgPath`, so even a caller that skips
  the CLI cannot get an operand read as a flag — the toolchain answers a
  leading dash with "malformed import path: leading dash".
- `internal/build`: a new `toolchainEnv` replaces bare `os.Environ()` in
  `archEnv` and in `requireMainPackage`'s env, dropping `GOFLAGS`. That
  covers every `go` compile gosd runs — the app, gosd-init and gosd-tsfunnel
  all go through `archEnv`. `GOPROXY`/`GOPRIVATE` and the other
  module-fetch variables are deliberately kept: they select where source
  comes from, which corporate proxies and offline caches legitimately need
  to control, and none of them names a program to run.

Tests: `cmd/gosd/pkgpath_test.go` drives the real cobra commands and asserts
both `build` and `run` reject the injected flag and that the payload's marker
was never created; `internal/build/injection_test.go` does the same one layer
down for the argv and `GOFLAGS` vectors, and a table pins the package-path
shapes that must keep working so the refusal can't quietly over-reach.
