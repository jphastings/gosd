---
# gosd-jc24
title: gosd build -- -toolexec=... executes arbitrary code on the build host (argument injection)
status: todo
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

- [ ] `--` terminator before `pkgPath` in `CrossCompile` and `requireMainPackage`
- [ ] Reject a leading-dash `pkgPath` at the CLI boundary with a clear error
- [ ] Strip `GOFLAGS` in `archEnv`
- [ ] Check `buildexternal.go` / `buildkernel.go` for the same trailing-operand shape
- [ ] Test: `gosd build -- -toolexec=<marker script>` fails, and the marker is not created
