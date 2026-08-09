---
# gosd-jm2v
title: Go toolchain preflight + record why the flake embeds Go
status: in-progress
type: task
priority: normal
created_at: 2026-08-09T12:00:41Z
updated_at: 2026-08-09T12:22:28Z
---

JP asked whether `flake.nix` should embed its own Go or depend on the user's
Go with a declared minimum. The question splits in two, with different
answers, and the change actually worth making is in neither place.

## Findings

**Build-time Go (compiling gosd inside `nix build`) — embedding is not a
choice.** nixpkgs' `buildGoModule` sets `GOTOOLCHAIN = "local"`
(`pkgs/build-support/go/module.nix`), and a nix sandbox has no user PATH, so
there is no "user's go" to defer to. The only knob is *which* nixpkgs Go, and
the margin is currently zero: go.mod's floor is `go 1.26.5` and `pkgs.go` at
the pinned nixpkgs rev (38a4887, nixos-unstable 2026-07-27) is exactly 1.26.5.

**That floor cannot be relaxed to `go 1.26`.** It isn't hand-set:
`go list -m -f '{{.Path}} {{.GoVersion}}' all` shows `tailscale.com` declares
`go 1.26.5` itself, and `go mod tidy` raises our directive to the maximum of
all dependency floors. Editing it down would be reverted by the next tidy, and
would genuinely fail to build tailscale.com.

**Runtime Go (gosd shelling out to compile the user's app and gosd-init) —
the bundle stays, as a fallback.** README sells
`nix run github:jphastings/gosd -- build ./cmd/myapp` as working offline on a
bare machine, and gosd-init's source ships vendored at `$out/share/gosd-src`
precisely so that build needs no network. `--suffix` (user's Go wins, bundled
Go is the fallback) is retained deliberately: gosd's job is compiling *your*
app, so a user who installed a newer Go for newer language features keeps it.

**The real gap is in gosd, not the flake.** gosd has no Go-version handling at
all — `internal/build` shells out to `go` at four sites and never checks the
toolchain exists or is new enough. With no Go on PATH a user gets
`exec: "go": executable file not found in $PATH` wrapped in `could not inspect
package X; try running 'go list X' directly to reproduce` — advice that fails
the same way, for a reason the message never names.

## Locked decisions

1. Keep the bundled Go in both roles. `--suffix` stays (user's Go wins);
   `pkgs.go` stays (no pin to `pkgs.go_1_26`, no overridden derivation).
2. **No up-front version comparison.** `GOTOOLCHAIN=auto` is Go's default and
   transparently fetches a newer toolchain, so comparing versions before
   building would reject setups that genuinely work. Preflight checks only
   that `go` *exists*; the version verdict is Go's own, and we make it
   readable by recognising Go's floor error in stderr and appending
   remediation.
3. `MinGoVersion` is quoted in user-facing text only, and is pinned to go.mod's
   directive by a test — not used for any comparison.
4. When go.mod's `go` directive rises, `nix flake update` belongs in the SAME
   PR. If nixos-unstable hasn't shipped that Go patch release yet, the nix job
   stays red until it does — a wait, not a broken change. This already cost
   PR #231 (`go: go.mod requires go >= 1.26.5 (running go 1.26.4;
   GOTOOLCHAIN=local)`), fixed by 599b3f7 bumping flake.lock; the coupling was
   written down nowhere.

## Todos

- [x] `internal/build/toolchain.go`: `MinGoVersion`, `CheckToolchain`, `explainBuildFailure`
- [x] Route `build.go` + `gosdinit.go` error sites through `explainBuildFailure`
- [x] Call `build.CheckToolchain()` from `cmd/gosd/build.go`'s validate sequence
- [x] Behavioral tests (go.mod pin, missing-go, floor-error remediation)
- [x] `flake.nix`: comment the two Go roles, the flake.lock coupling, and fix the stale `-trimpath` claim
- [x] `CLAUDE.md`: locked decision
- [x] Quality gates green, PR opened

## Note: a stale comment in flake.nix

Lines 48-57 claim `-trimpath` erases the compiled-from checkout path, defeating
the dev-checkout rung. But `buildGoModule` only adds `-trimpath` when
`allowGoReference` is unset (`lib.optional (!finalAttrs.allowGoReference)
"-trimpath"`), and the flake sets `allowGoReference = true`. The rung still
fails — `runtime.Caller(0)` yields the sandbox's `/build/source/…`, absent on
the user's machine — but for that reason, not the stated one.


## Summary of Changes

**`internal/build/toolchain.go` (new)** — `MinGoVersion` (pinned to go.mod's
directive by a test, quoted in exactly one error and never compared),
`CheckToolchain()` (an `exec.LookPath` existence check only), and
`explainBuildFailure(what, reproduce, stderr)`, the shared wrapper the
package's three `go` error sites now use.

**Error sites routed through the helper** — `CrossCompile` and
`requireMainPackage` in `build.go`, `crossCompileInDir` in `gosdinit.go`
(which covers `CrossCompileTsfunnel` for free). Each site keeps its own
complete `what` clause, so baseline wording is byte-identical to before; the
helper only adds the floor remediation. No argv or env construction was
touched — `buildGoBuildArgs`/`archEnv` are untouched, preserving gosd-init's
byte-identical-argv invariant.

**`cmd/gosd/build.go`** — `build.CheckToolchain()` runs last in the guard
sequence, after `validateIngressDataPartition`, so a flag mistake is reported
before an environment one.

**Amendment to decision 2 during review:** the remediation paragraph quotes no
version of its own. The first draft named `MinGoVersion`, which is wrong for
`CrossCompile`: it builds the *user's* main package, whose floor may exceed
gosd's, so the appended line would have contradicted the Go stderr printed
directly above it (stderr says `requires go >= 1.27.0`, our line would have
said `1.26.5`). Go's own stderr already names both the required and running
versions, so the remediation just points at the fix.
`TestExplainBuildFailureQuotesNoVersionOfItsOwn` pins this.

**`flake.nix`** — comments only, no functional change. Documents the two Go
roles, why `--suffix` beats `--prefix`, and the flake.lock coupling. Also
corrects a stale claim: the dev-checkout rung does not fail because of
`-trimpath` (`buildGoModule` adds that only when `allowGoReference` is unset,
and the flake sets it `true`) but because `runtime.Caller` resolves to the
sandbox build directory, gone by the time a user runs gosd.

**`CLAUDE.md`** — the locked decision, under project-wide decisions.

### Verified

Before, with no `go` on PATH, gosd got as far as printing partition labels and
then failed with an empty stderr and advice that would fail the same way:

```
gosd build: boot partition label: hello-boot
gosd: cross-compiling ./examples/hello for qemu-virt failed: could not inspect
package ./examples/hello; try running `go list ./examples/hello` directly to
reproduce:
```

After:

```
gosd: gosd needs a Go toolchain of at least go1.26.5 to compile your app and
gosd-init, but none was found on PATH; install one from https://go.dev/dl, or
try `nix run github:jphastings/gosd`, which bundles a suitable toolchain
```

Gates run in the worktree: `go build ./...`, `go test ./...` (no failures),
`go vet ./...`, `gofmt -l .` (silent), `golangci-lint run ./...` and
`GOOS=linux golangci-lint run ./...` (both `0 issues.`). `js/` untouched.
`nix build` not run locally (nix isn't installed on this machine) — the
flake edits are comments only, and CI's `nix-build` job is the real gate.
