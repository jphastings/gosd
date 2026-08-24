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

// AppCompileOptions are the optional `go build` flags CrossCompile forwards
// to the app's own compile, mirroring the flags a Go developer already knows
// from `go build` itself (--ldflags, --tags, --trimpath, --gcflags,
// --asmflags on `gosd build`), so existing muscle memory works unchanged.
//
// This is a new, purpose-built pair with appGoBuildArgs, deliberately not a
// reuse of crossCompileOpts/buildGoBuildArgs (gosdinit.go): those always
// emit -C <dir> and resolve an absolute output path, both specific to
// building gosd-init/tsfunnel from a different, detected/downloaded source
// tree. The app always builds from the caller's own working directory,
// using pkgPath/outputPath exactly as given.
type AppCompileOptions struct {
	// Tags is passed to `go build` as `-tags <Tags>` when non-empty - gosd
	// uses this to pass boards.BuildTags (merged with any caller-supplied
	// --tags, see cmd/gosd's parseExtraTags) so a developer's app can gate
	// source on being compiled by gosd at all (`//go:build gosd`) and on
	// the board it's being compiled for (`//go:build gosd_<id>`).
	Tags string
	// LDFlags is passed to `go build` as `-ldflags <LDFlags>` when
	// non-empty, e.g. "-X main.version=1.4.2" to stamp a version into the
	// compiled binary.
	LDFlags string
	// GCFlags is passed to `go build` as `-gcflags <GCFlags>` when
	// non-empty.
	GCFlags string
	// ASMFlags is passed to `go build` as `-asmflags <ASMFlags>` when
	// non-empty.
	ASMFlags string
	// TrimPath passes `-trimpath` to `go build` when true.
	TrimPath bool
}

// CrossCompile builds the Go main package at pkgPath into a static binary
// for arch at outputPath, by shelling out to the host Go toolchain, applying
// opts's flags to the invocation (see AppCompileOptions). It fails with an
// actionable error if pkgPath is not a main package, or if the build itself
// fails; in the latter case the compiler's stderr is included verbatim.
func CrossCompile(pkgPath, outputPath string, opts AppCompileOptions, arch boards.Arch) error {
	if err := requireMainPackage(pkgPath); err != nil {
		return err
	}

	args := appGoBuildArgs(outputPath, pkgPath, opts)
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

// appGoBuildArgs assembles CrossCompile's `go` argv as a pure function of
// its inputs, so its flag ordering is unit-testable without shelling out.
// Flags are appended in a fixed order (-tags, -ldflags, -gcflags,
// -asmflags, -trimpath) whenever opts sets them, always before the "--"
// terminator and pkgPath. "--" is what stops the toolchain reading pkgPath
// as another build flag: without it a pkgPath of "-toolexec=/tmp/payload"
// would run an arbitrary program in place of the compiler on the build host
// (bean gosd-jc24). cmd/gosd rejects such a pkgPath before it reaches here;
// this terminator is what makes that a second line of defence rather than
// the only one.
func appGoBuildArgs(outputPath, pkgPath string, opts AppCompileOptions) []string {
	args := []string{"build", "-o", outputPath}
	if opts.Tags != "" {
		args = append(args, "-tags", opts.Tags)
	}
	if opts.LDFlags != "" {
		args = append(args, "-ldflags", opts.LDFlags)
	}
	if opts.GCFlags != "" {
		args = append(args, "-gcflags", opts.GCFlags)
	}
	if opts.ASMFlags != "" {
		args = append(args, "-asmflags", opts.ASMFlags)
	}
	if opts.TrimPath {
		args = append(args, "-trimpath")
	}
	return append(args, "--", pkgPath)
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
