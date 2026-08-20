---
gosd: major
---

#### `gosd build` and `gosd run` refuse a package path that is really a build flag

A package path starting with `-` is no longer passed to the Go toolchain, and
gosd's own `go` invocations now pass Go's `--` terminator before it, so one can
never be read as a build flag again.

This closes a way of getting arbitrary code to run on the machine doing the
build. `gosd build -- -toolexec=/tmp/payload` reached `go build` with
`-toolexec` intact, and `-toolexec` runs a program of the caller's choosing in
place of the compiler — before the app was even compiled, and so with control
over every image that build produced. Reaching it needs influence over gosd's
arguments, which is exactly what a wrapper forwarding a value it does not fully
control gives away: a CI job templating a branch-derived path, a `Makefile`'s
`gosd build $(PKG)`, a script taking a package argument.

An ambient `GOFLAGS` is no longer inherited by gosd's `go` subprocesses either.
It can carry `-toolexec` just as well, and needs no control over gosd's
arguments at all — a `.envrc` picked up on entering a cloned repository, a
modified shell profile, or an inherited CI variable was enough. `GOPROXY`,
`GOPRIVATE` and the other module-fetch variables are still honoured.

Anything that isn't recognisably a package path — a relative path, an absolute
path, or an import path — is now refused with an error naming what was rejected
and what a valid argument looks like. Every documented invocation, `gosd build
.` included, is unaffected.
