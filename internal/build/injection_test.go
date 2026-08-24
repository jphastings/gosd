package build

import (
	"os"
	"path/filepath"
	"testing"
)

// toolexecPayload writes a program of the shape an attacker would supply to
// -toolexec: it records having run by creating a marker file, then execs the
// real compiler so the build carries on looking normal. It returns the
// program's path and the marker path whose existence proves it ran.
func toolexecPayload(t *testing.T) (program, marker string) {
	t.Helper()

	dir := t.TempDir()
	program = filepath.Join(dir, "payload.sh")
	marker = filepath.Join(dir, "executed")
	script := "#!/bin/sh\ntouch " + marker + "\nexec \"$@\"\n"
	if err := os.WriteFile(program, []byte(script), 0o755); err != nil {
		t.Fatalf("writing the payload program: %v", err)
	}
	return program, marker
}

func markerExists(t *testing.T, marker string) bool {
	t.Helper()

	_, err := os.Stat(marker)
	return err == nil
}

// TestCrossCompileDoesNotRunAFlagShapedPackagePath is the regression test for
// bean gosd-jc24: `gosd build -- -toolexec=/tmp/payload` reached here with
// pkgPath == "-toolexec=/tmp/payload", and without Go's own "--" terminator
// in the constructed argv the toolchain read it as a build flag and ran the
// payload in place of the compiler.
//
// The test runs from a directory that does hold a main package, because that
// is what let the original exploit through: `go list` given only flags falls
// back to the working directory's package, so the requireMainPackage
// preflight saw a valid "main" and the build proceeded.
func TestCrossCompileDoesNotRunAFlagShapedPackagePath(t *testing.T) {
	program, marker := toolexecPayload(t)
	out := filepath.Join(t.TempDir(), "out")
	t.Chdir("./testdata/hello")

	if err := CrossCompile("-toolexec="+program, out, AppCompileOptions{}, arm64); err == nil {
		t.Error("CrossCompile accepted a flag-shaped package path, want an error")
	}

	if markerExists(t, marker) {
		t.Fatal("the -toolexec payload ran: a package path starting with \"-\" reached the Go toolchain as a build flag")
	}
}

// TestCrossCompileIgnoresAmbientGOFLAGS covers gosd-jc24's second vector,
// which needs no control over gosd's argv at all: a GOFLAGS set by a repo's
// .envrc, a shell profile or an inherited CI variable used to be passed
// straight through to the compiler.
func TestCrossCompileIgnoresAmbientGOFLAGS(t *testing.T) {
	program, marker := toolexecPayload(t)
	t.Setenv("GOFLAGS", "-toolexec="+program)

	if err := CrossCompile("./testdata/hello", filepath.Join(t.TempDir(), "out"), AppCompileOptions{}, arm64); err != nil {
		t.Fatalf("CrossCompile: %v", err)
	}

	if markerExists(t, marker) {
		t.Fatal("the -toolexec payload ran: an ambient GOFLAGS reached the compiler")
	}
}
