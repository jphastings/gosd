// Package build cross-compiles Go packages into the static linux/arm64
// binaries that end up on a gosd image (the user's app, and gosd-init).
package build

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Target platform for every gosd build. GoSD only ever targets arm64 Linux
// boards, and CGO is always disabled so the result never depends on the
// host's C library.
const (
	targetGOOS   = "linux"
	targetGOARCH = "arm64"
)

// CrossCompile builds the Go main package at pkgPath into a static
// linux/arm64 binary at outputPath, by shelling out to the host Go
// toolchain. It fails with an actionable error if pkgPath is not a main
// package, or if the build itself fails; in the latter case the compiler's
// stderr is included verbatim.
func CrossCompile(pkgPath, outputPath string) error {
	if err := requireMainPackage(pkgPath); err != nil {
		return err
	}

	cmd := exec.Command("go", "build", "-o", outputPath, pkgPath)
	cmd.Env = append(os.Environ(),
		"CGO_ENABLED=0",
		"GOOS="+targetGOOS,
		"GOARCH="+targetGOARCH,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("building %s for %s/%s failed; try running `go build %s` directly to reproduce:\n%s",
			pkgPath, targetGOOS, targetGOARCH, pkgPath, stderr.String())
	}
	return nil
}

func requireMainPackage(pkgPath string) error {
	cmd := exec.Command("go", "list", "-f", "{{.Name}}", pkgPath)
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
