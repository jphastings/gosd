---
# gosd-7frv
title: 'gosd version: report the CLI version and which artifacts it will download'
status: todo
type: feature
created_at: 2026-08-17T12:50:00Z
updated_at: 2026-08-17T12:50:00Z
---

`gosd` has no `version` command and no `--version` flag. The only way to know
what you have installed is:

```
go version -m $(which gosd) | grep '\smod\s'
	mod	github.com/jphastings/gosd	v0.6.2
```

...which needs the Go toolchain, and still doesn't answer the question that
actually matters: **which board artifacts will this binary download?**

Found the hard way (2026-08-17): an image built with an installed `gosd`
wouldn't boot, and diagnosing it meant reading the SPL build date off a
serial capture, matching it against the artifact cache, then checking the
release tag's source to learn that `v0.6.2` pins artifacts `v0.10.0`. A
`gosd version` printing both would have answered it immediately.

## What it should print

- The CLI version — from `runtime/debug.ReadBuildInfo`, so it works for
  `go install`ed binaries (module version) and local builds (`(devel)` plus
  the VCS revision and dirty flag when stamped).
- **The artifacts pin** (`internal/artifacts.Version`), which is the
  build-affecting fact and the reason this bean exists.
- The Go toolchain version, cheap and often relevant to a build problem.

Keep it plain text, one fact per line, greppable. A `--version` flag on the
root command should print the same thing, since that is what people try
first.

## Todos

- [x] `gosd version` subcommand plus a root `--version` flag
- [x] Behavioural tests: output names the artifacts pin, and degrades
      gracefully when build info is unavailable rather than crashing
- [x] Mention it where a user is told to check what they have installed
      (docs/artifacts.md's pin discussion, or the troubleshooting docs)


## Summary of Changes

`gosd version` (and `gosd --version`, which prints the same, since that is
what people try first) reports three lines: the CLI version, **the artifacts
pin**, and the Go toolchain.

The artifacts line is the point. A binary built from a checkout also reports
its commit and whether the tree was modified.

Version data comes from `runtime/debug.ReadBuildInfo`, so it is right for
`go install`ed binaries without any ldflags plumbing at build time. Missing
build information degrades to `unknown (built without version information)`
rather than failing — a version command that errors is less useful than one
that admits what it cannot see. Tests cover the pin appearing in the output,
`--version` matching the subcommand, the no-build-info path, and a dirty
checkout being reported as modified.

docs/artifacts.md's pinning section now shows the command, and says plainly
that a release pins the artifacts that existed when it was cut — which is the
trap that motivated this.
