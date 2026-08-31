package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// toolexecPayloadArg writes a program of the shape an attacker would supply
// to -toolexec (it records having run, then execs the real compiler so the
// build looks normal) and returns the whole `-toolexec=...` argument plus the
// marker path whose existence proves the program ran.
func toolexecPayloadArg(t *testing.T) (arg, marker string) {
	t.Helper()

	dir := t.TempDir()
	program := filepath.Join(dir, "payload.sh")
	marker = filepath.Join(dir, "executed")
	script := "#!/bin/sh\ntouch " + marker + "\nexec \"$@\"\n"
	if err := os.WriteFile(program, []byte(script), 0o755); err != nil {
		t.Fatalf("writing the payload program: %v", err)
	}
	return "-toolexec=" + program, marker
}

// TestFlagShapedPackagePathIsRejected is the regression test for bean
// gosd-jc24. cobra honours "--" as a flag terminator, so `gosd build --
// -toolexec=/tmp/payload` used to land the whole build flag in args[0] and
// carry it, unvalidated, into `go build` - where -toolexec runs an arbitrary
// program on the build host in place of the compiler. Both subcommands that
// take a package path shared the hole, so both must refuse it, and neither
// may have run the payload by the time it does.
func TestFlagShapedPackagePathIsRejected(t *testing.T) {
	for _, subcommand := range []string{"build", "run", "init"} {
		t.Run(subcommand, func(t *testing.T) {
			arg, marker := toolexecPayloadArg(t)

			cmd := newRootCmd()
			cmd.SetArgs([]string{subcommand, "--", arg})
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

			err := cmd.Execute()
			if err == nil {
				t.Fatalf("gosd %s accepted a flag-shaped package path, want an error", subcommand)
			}
			if !strings.Contains(err.Error(), arg) {
				t.Errorf("error %q does not name the rejected argument %q", err, arg)
			}
			if !strings.Contains(err.Error(), "main package") {
				t.Errorf("error %q does not say what a valid argument looks like", err)
			}

			if _, statErr := os.Stat(marker); statErr == nil {
				t.Fatalf("gosd %s ran the -toolexec payload before rejecting it", subcommand)
			}
		})
	}
}

// TestPackagePathsGosdDocumentsAreAccepted keeps the gosd-jc24 refusal from
// over-reaching: every package path shape gosd's own docs and examples use
// must still get through.
func TestPackagePathsGosdDocumentsAreAccepted(t *testing.T) {
	accepted := []string{
		".",
		"..",
		"./cmd/myapp",
		"../sibling/cmd/myapp",
		"/Users/someone/src/My App/cmd/myapp",
		"github.com/jphastings/gosd/examples/hello",
		"_local/app",
	}
	for _, pkgPath := range accepted {
		if err := validatePkgPath(pkgPath); err != nil {
			t.Errorf("validatePkgPath(%q) = %v, want it accepted", pkgPath, err)
		}
	}

	refused := []string{
		"",
		"-toolexec=/tmp/payload",
		"-ldflags=-X main.x=y",
		"--overlay=/tmp/overlay.json",
		"-",
	}
	for _, pkgPath := range refused {
		if err := validatePkgPath(pkgPath); err == nil {
			t.Errorf("validatePkgPath(%q) = nil, want it refused", pkgPath)
		}
	}
}
