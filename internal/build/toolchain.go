package build

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// MinGoVersion is the minimum Go toolchain gosd can drive to cross-compile a
// user's app and gosd-init. It mirrors go.mod's own `go` directive, which is
// dependency-derived - `go mod tidy` raises it to the max of every
// dependency's floor (tailscale.com sets today's value) - so this constant
// exists only to be quoted in CheckToolchain's error, the one place gosd has
// to name a version with no `go` around to name it for us;
// TestMinGoVersionMatchesGoMod keeps it honest against go.mod.
//
// Do not use this constant for any version comparison: see CheckToolchain for
// why.
const MinGoVersion = "go1.26.5"

// CheckToolchain reports whether a `go` binary is on PATH at all. It
// deliberately does not compare versions - GOTOOLCHAIN=auto is Go's default
// and transparently fetches a newer toolchain on demand, so rejecting an
// older `go` up front would reject setups that genuinely work. The `go`
// command itself is the authority on whether it can satisfy go.mod; this
// only catches the one failure mode it can't self-report: no `go` at all.
func CheckToolchain() error {
	if _, err := exec.LookPath("go"); err != nil {
		return fmt.Errorf(
			"gosd needs a Go toolchain of at least %s to compile your app and gosd-init, but none was found on PATH; "+
				"install one from https://go.dev/dl, or try `nix run github:jphastings/gosd`, which bundles a suitable toolchain",
			MinGoVersion)
	}
	return nil
}

// goToolchainFloor is the fragment common to both shapes of Go's own
// toolchain-floor complaint:
//
//	go: go.mod requires go >= 1.26.5 (running go 1.26.4; GOTOOLCHAIN=local)
//	go: module tailscale.com@v1.102.2 requires go >= 1.26.5 (running go 1.26.4)
const goToolchainFloor = "requires go >= "

// explainBuildFailure is the shared shape every `go build`/`go list` error
// site in this package wraps its stderr in: "<what>; try running `<reproduce>`
// directly to reproduce:\n<stderr>". what is each call site's own complete
// clause (e.g. "building ./cmd/myapp for linux/arm64 failed", or
// requireMainPackage's "could not inspect package ./cmd/myapp", which has no
// "failed" of its own) so every site's baseline wording stays byte-identical
// to what it was before this helper existed.
//
// When stderr carries Go's toolchain-floor complaint, a remediation paragraph
// is appended. It quotes no version of its own - deliberately: the floor that
// tripped may belong to the user's app rather than to gosd (CrossCompile
// builds their main package, which may require a newer Go than MinGoVersion),
// and Go's own stderr sitting directly above already names both the required
// and the running version. Naming MinGoVersion here would contradict it.
func explainBuildFailure(what, reproduce, stderr string) error {
	msg := fmt.Sprintf("%s; try running `%s` directly to reproduce:\n%s", what, reproduce, stderr)
	if strings.Contains(stderr, goToolchainFloor) {
		msg += "\nThe Go toolchain on your PATH is older than the module being built requires. " +
			"Upgrade Go from https://go.dev/dl, or let Go fetch a suitable toolchain itself - " +
			"that is what the default GOTOOLCHAIN=auto does, so check whether something in your " +
			"environment has set GOTOOLCHAIN=local."
	}
	return errors.New(msg)
}
