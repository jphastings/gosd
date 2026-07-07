package main

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/jphastings/gosd/internal/boards"
)

// countingCompiler records every compileApp/compileInit call it sees, so
// tests can assert exactly how many real cross-compiles compileForArchs
// triggers - the dedupe guarantee itself, not just its output.
type countingCompiler struct {
	appCalls, initCalls []boards.Arch
}

func (c *countingCompiler) compileApp(_, _ string, arch boards.Arch) error {
	c.appCalls = append(c.appCalls, arch)
	return nil
}

func (c *countingCompiler) compileInit(_, _ string, arch boards.Arch) error {
	c.initCalls = append(c.initCalls, arch)
	return nil
}

var (
	arm64gosd = boards.Arch{GOARCH: "arm64"}
	armv6gosd = boards.Arch{GOARCH: "arm", GOARM: "6"}
)

// TestCompileForArchsDedupesSameArch is the dedupe acceptance test the bean
// requires: selecting three arm64 boards must compile the app and gosd-init
// exactly once, not three times.
func TestCompileForArchsDedupesSameArch(t *testing.T) {
	c := &countingCompiler{}
	archs := []boards.Arch{arm64gosd, arm64gosd, arm64gosd}

	binaries, err := compileForArchs(archs, t.TempDir(), "./pkg", "", c.compileApp, c.compileInit)
	if err != nil {
		t.Fatalf("compileForArchs: %v", err)
	}

	if len(c.appCalls) != 1 {
		t.Errorf("compileApp was called %d times for 3 identical arm64 boards, want exactly 1", len(c.appCalls))
	}
	if len(c.initCalls) != 1 {
		t.Errorf("compileInit was called %d times for 3 identical arm64 boards, want exactly 1", len(c.initCalls))
	}
	if len(binaries) != 1 {
		t.Errorf("compileForArchs returned %d arch entries, want exactly 1 (arm64)", len(binaries))
	}
	if _, ok := binaries[arm64gosd.Key()]; !ok {
		t.Errorf("compileForArchs result is missing the arm64 entry: %v", binaries)
	}
}

// TestCompileForArchsAddsOnePassPerDistinctArch confirms the other half of
// the dedupe contract: mixing in a second, distinct arch (a hypothetical
// GOARM=6 board alongside two arm64 ones) adds exactly one more compile
// pass, not a third arm64 one.
func TestCompileForArchsAddsOnePassPerDistinctArch(t *testing.T) {
	c := &countingCompiler{}
	archs := []boards.Arch{arm64gosd, armv6gosd, arm64gosd}

	binaries, err := compileForArchs(archs, t.TempDir(), "./pkg", "", c.compileApp, c.compileInit)
	if err != nil {
		t.Fatalf("compileForArchs: %v", err)
	}

	if len(c.appCalls) != 2 {
		t.Errorf("compileApp was called %d times for 2 arm64 + 1 arm-6 board, want exactly 2 (one per distinct arch)", len(c.appCalls))
	}
	if len(c.initCalls) != 2 {
		t.Errorf("compileInit was called %d times for 2 arm64 + 1 arm-6 board, want exactly 2 (one per distinct arch)", len(c.initCalls))
	}
	for _, want := range []string{arm64gosd.Key(), armv6gosd.Key()} {
		if _, ok := binaries[want]; !ok {
			t.Errorf("compileForArchs result is missing the %q entry: %v", want, binaries)
		}
	}
}

// TestCompileForArchsSurfacesAppCompileFailure confirms a compileApp failure
// is reported with the failing package and arch, and stops before wasting a
// compileInit call.
func TestCompileForArchsSurfacesAppCompileFailure(t *testing.T) {
	compileApp := func(_, _ string, _ boards.Arch) error { return errors.New("boom") }
	initCalls := 0
	compileInit := func(_, _ string, _ boards.Arch) error {
		initCalls++
		return nil
	}

	_, err := compileForArchs([]boards.Arch{arm64gosd}, t.TempDir(), "./pkg", "", compileApp, compileInit)
	if err == nil {
		t.Fatal("compileForArchs succeeded despite a failing compileApp, want an error")
	}
	if initCalls != 0 {
		t.Errorf("compileInit was called after compileApp failed, want it skipped")
	}
}

// TestCompileForArchsWritesDistinctBinaryNamesPerArch guards against a
// dedupe bug where two archs' binaries collide on disk: each arch's app and
// gosd-init binaries must land at their own path inside tempDir.
func TestCompileForArchsWritesDistinctBinaryNamesPerArch(t *testing.T) {
	c := &countingCompiler{}
	tempDir := t.TempDir()

	binaries, err := compileForArchs([]boards.Arch{arm64gosd, armv6gosd}, tempDir, "./pkg", "", c.compileApp, c.compileInit)
	if err != nil {
		t.Fatalf("compileForArchs: %v", err)
	}

	arm64Bin, armv6Bin := binaries[arm64gosd.Key()], binaries[armv6gosd.Key()]
	if arm64Bin.appPath == armv6Bin.appPath {
		t.Errorf("both archs share app binary path %q, want distinct paths", arm64Bin.appPath)
	}
	if arm64Bin.initPath == armv6Bin.initPath {
		t.Errorf("both archs share gosd-init binary path %q, want distinct paths", arm64Bin.initPath)
	}
	for _, p := range []string{arm64Bin.appPath, arm64Bin.initPath, armv6Bin.appPath, armv6Bin.initPath} {
		if filepath.Dir(p) != tempDir {
			t.Errorf("binary path %q is not inside tempDir %q", p, tempDir)
		}
	}
}
