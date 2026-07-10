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
// pass a board's boards.BuildTag so a developer's app can gate board-
// specific source with `//go:build gosd_<id>`; an empty tags builds with no
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
	args = append(args, pkgPath)

	cmd := exec.Command("go", args...)
	cmd.Env = archEnv(arch)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("building %s for %s/%s failed; try running `go build %s` directly to reproduce:\n%s",
			pkgPath, targetGOOS, arch.GOARCH, pkgPath, stderr.String())
	}
	return nil
}

// archEnv returns the env every gosd cross-compile runs with: the host's own
// environment plus CGO disabled and GOOS/GOARCH/GOARM set for arch (GOARM
// omitted when arch doesn't set one, e.g. arm64).
func archEnv(arch boards.Arch) []string {
	env := append(os.Environ(),
		"CGO_ENABLED=0",
		"GOOS="+targetGOOS,
		"GOARCH="+arch.GOARCH,
	)
	if arch.GOARM != "" {
		env = append(env, "GOARM="+arch.GOARM)
	}
	return env
}

func requireMainPackage(pkgPath string) error {
	cmd := exec.Command("go", "list", "-f", "{{.Name}}", pkgPath)
	// Inspect the package under the same GOOS every gosd build actually
	// targets (targetGOOS, always "linux"), not the host's own GOOS: a
	// package gated with a `//go:build linux` tag (as a dependency on a
	// Linux-only chardev API can force an example to be, e.g.
	// examples/gpioinfo) is a real main package under the build gosd
	// performs, even though `go list` would otherwise report it has no Go
	// files at all when run unmodified on a macOS host.
	cmd.Env = append(os.Environ(), "GOOS="+targetGOOS)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("could not inspect package %s; try running `go list %s` directly to reproduce:\n%s",
			pkgPath, pkgPath, stderr.String())
	}

	name := strings.TrimSpace(stdout.String())
	if name != "main" {
		return fmt.Errorf("%s is package %q, not \"main\"; gosd build requires a runnable command (package main with a func main)", pkgPath, name)
	}
	return nil
}
