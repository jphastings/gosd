// Package build cross-compiles Go packages into the static Linux binaries
// that end up on a gosd image (the user's app, and gosd-init), targeting
// whichever GOARCH/GOARM a board's boards.Arch calls for.
package build

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/jphastings/gosd/internal/boards"
)

// targetGOOS is the OS every gosd build targets; only GOARCH/GOARM vary per
// board (see boards.Arch). CGO is always disabled so the result never
// depends on the host's C library.
const targetGOOS = "linux"

// CrossCompile builds the Go main package at pkgPath into a static binary
// for arch at outputPath, by shelling out to the host Go toolchain. tags, if
// non-empty, is passed to `go build` as `-tags <tags>` - gosd uses this to
// pass boards.BuildTags so a developer's app can gate source on being
// compiled by gosd at all (`//go:build gosd`) and on the board it's being
// compiled for (`//go:build gosd_<id>`); an empty tags builds with no
// extra build tags at all. It fails with an actionable error if pkgPath is
// not a main package, or if the build itself fails; in the latter case the
// compiler's stderr is included verbatim.
func CrossCompile(pkgPath, outputPath, tags string, arch boards.Arch) error {
	if err := requireMainPackage(pkgPath); err != nil {
		return err
	}

	args := []string{"build", "-o", outputPath}
	if tags != "" {
		args = append(args, "-tags", tags)
	}
	// "--" is what stops the toolchain reading pkgPath as another build
	// flag: without it a pkgPath of "-toolexec=/tmp/payload" would run an
	// arbitrary program in place of the compiler on the build host (bean
	// gosd-jc24). cmd/gosd rejects such a pkgPath before it reaches here;
	// this terminator is what makes that a second line of defence rather
	// than the only one.
	args = append(args, "--", pkgPath)

	cmd := exec.Command("go", args...)
	cmd.Env = archEnv(arch)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return explainBuildFailure(
			fmt.Sprintf("building %s for %s/%s failed", pkgPath, targetGOOS, arch.GOARCH),
			fmt.Sprintf("go build %s", pkgPath),
			stderr.String())
	}
	return nil
}

// archEnv returns the env every gosd cross-compile runs with: toolchainEnv
// plus CGO disabled and GOOS/GOARCH/GOARM set for arch (GOARM omitted when
// arch doesn't set one, e.g. arm64).
func archEnv(arch boards.Arch) []string {
	env := append(toolchainEnv(),
		"CGO_ENABLED=0",
		"GOOS="+targetGOOS,
		"GOARCH="+arch.GOARCH,
	)
	if arch.GOARM != "" {
		env = append(env, "GOARM="+arch.GOARM)
	}
	return env
}

// toolchainEnv is the host environment every `go` subprocess gosd starts
// inherits: os.Environ() with GOFLAGS removed. GOFLAGS can carry -toolexec,
// -ldflags and friends, so an ambient one - a direnv .envrc picked up on
// entering a cloned repo, a poisoned shell profile, an inherited CI variable
// - would otherwise reach the compiler and execute arbitrary code on the
// build host without the attacker ever touching gosd's argv (bean
// gosd-jc24). Nothing gosd builds needs a caller's GOFLAGS: every flag that
// matters is set explicitly here and in each command's argv. GOPROXY,
// GOPRIVATE and the other module-fetch variables deliberately stay - they
// select where source is fetched from, which corporate proxies and offline
// caches legitimately need to control, and none of them names a program to
// run.
func toolchainEnv() []string {
	env := os.Environ()
	kept := make([]string, 0, len(env))
	for _, kv := range env {
		if strings.HasPrefix(kv, "GOFLAGS=") {
			continue
		}
		kept = append(kept, kv)
	}
	return kept
}

func requireMainPackage(pkgPath string) error {
	// "--" for the same reason CrossCompile passes it: pkgPath must reach
	// the toolchain as an operand, never as a flag.
	cmd := exec.Command("go", "list", "-f", "{{.Name}}", "--", pkgPath)
	// Inspect the package under the same GOOS every gosd build actually
	// targets (targetGOOS, always "linux"), not the host's own GOOS: a
	// package gated with a `//go:build linux` tag (as a dependency on a
	// Linux-only chardev API can force an example to be, e.g.
	// examples/gpioinfo) is a real main package under the build gosd
	// performs, even though `go list` would otherwise report it has no Go
	// files at all when run unmodified on a macOS host.
	cmd.Env = append(toolchainEnv(), "GOOS="+targetGOOS)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return explainBuildFailure(
			fmt.Sprintf("could not inspect package %s", pkgPath),
			fmt.Sprintf("go list %s", pkgPath),
			stderr.String())
	}

	name := strings.TrimSpace(stdout.String())
	if name != "main" {
		return fmt.Errorf("%s is package %q, not \"main\"; gosd build requires a runnable command (package main with a func main)", pkgPath, name)
	}
	return nil
}
