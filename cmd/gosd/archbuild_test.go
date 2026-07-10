package main

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/jphastings/gosd/internal/boards"
	"github.com/jphastings/gosd/internal/boards/nanopizero2"
	"github.com/jphastings/gosd/internal/boards/pizero2w"
	"github.com/jphastings/gosd/internal/boards/pizerow"
	"github.com/jphastings/gosd/internal/boards/radxazero3e"
)

// appCall records one compileApp invocation's arguments, so tests can assert
// not just how many times it was called but with which board's tag.
type appCall struct {
	tags string
	arch boards.Arch
}

// countingCompiler records every compileApp/compileInit call it sees, so
// tests can assert exactly how many real cross-compiles compileForBoards
// triggers - the per-board app / per-arch init guarantee itself, not just
// its output.
type countingCompiler struct {
	appCalls  []appCall
	initCalls []boards.Arch
}

func (c *countingCompiler) compileApp(_, _, tags string, arch boards.Arch) error {
	c.appCalls = append(c.appCalls, appCall{tags: tags, arch: arch})
	return nil
}

func (c *countingCompiler) compileInit(_, _ string, arch boards.Arch) error {
	c.initCalls = append(c.initCalls, arch)
	return nil
}

var arm64gosd = boards.Arch{GOARCH: "arm64"}

// TestCompileForBoardsCompilesAppOncePerBoardOnSharedArch is the keystone
// test for gosd-1937: two boards sharing one arch (pi-zero-2w and
// radxa-zero-3e, both arm64) must each get their OWN app compile pass -
// tagged with their own boards.BuildTag - even though they share a single
// gosd-init compile pass (the pre-existing per-arch dedupe, unaffected by
// per-board app tagging).
func TestCompileForBoardsCompilesAppOncePerBoardOnSharedArch(t *testing.T) {
	c := &countingCompiler{}
	selected := []boards.Board{pizero2w.New(), radxazero3e.New()}

	binaries, err := compileForBoards(selected, t.TempDir(), "./pkg", "", c.compileApp, c.compileInit)
	if err != nil {
		t.Fatalf("compileForBoards: %v", err)
	}

	if len(c.appCalls) != 2 {
		t.Fatalf("compileApp was called %d times for 2 boards sharing one arch, want exactly 2 (one per board)", len(c.appCalls))
	}
	gotTags := map[string]bool{c.appCalls[0].tags: true, c.appCalls[1].tags: true}
	for _, b := range selected {
		want := boards.BuildTag(b)
		if !gotTags[want] {
			t.Errorf("compileApp tags = %v, want them to include %q (for board %q)", gotTags, want, b.Name())
		}
		if c.appCalls[0].arch != arm64gosd || c.appCalls[1].arch != arm64gosd {
			t.Errorf("compileApp arch = %+v / %+v, want both arm64", c.appCalls[0].arch, c.appCalls[1].arch)
		}
	}

	if len(c.initCalls) != 1 {
		t.Errorf("compileInit was called %d times for 2 boards sharing one arch, want exactly 1", len(c.initCalls))
	}

	for _, b := range selected {
		if _, ok := binaries[b.Name()]; !ok {
			t.Errorf("compileForBoards result is missing the %q entry: %v", b.Name(), binaries)
		}
	}
	if binaries[selected[0].Name()].initPath != binaries[selected[1].Name()].initPath {
		t.Errorf("boards sharing an arch got different initPaths (%q vs %q), want the same shared gosd-init binary",
			binaries[selected[0].Name()].initPath, binaries[selected[1].Name()].initPath)
	}
}

// TestCompileForBoardsAddsOneInitPassPerDistinctArch confirms the other half
// of the contract: mixing in a GOARM=6 board (pi-zero-w) alongside two arm64
// boards adds exactly one more gosd-init compile pass, and a third app
// compile pass (still one per board).
func TestCompileForBoardsAddsOneInitPassPerDistinctArch(t *testing.T) {
	c := &countingCompiler{}
	selected := []boards.Board{pizero2w.New(), nanopizero2.New(), pizerow.New()}

	binaries, err := compileForBoards(selected, t.TempDir(), "./pkg", "", c.compileApp, c.compileInit)
	if err != nil {
		t.Fatalf("compileForBoards: %v", err)
	}

	if len(c.appCalls) != 3 {
		t.Errorf("compileApp was called %d times for 3 boards, want exactly 3 (one per board)", len(c.appCalls))
	}
	if len(c.initCalls) != 2 {
		t.Errorf("compileInit was called %d times for 2 arm64 boards + 1 arm-6 board, want exactly 2 (one per distinct arch)", len(c.initCalls))
	}

	if binaries["pi-zero-2w"].initPath != binaries["nanopi-zero2"].initPath {
		t.Errorf("pi-zero-2w and nanopi-zero2 (both arm64) got different initPaths, want the same shared binary")
	}
	if binaries["pi-zero-2w"].initPath == binaries["pi-zero-w"].initPath {
		t.Errorf("pi-zero-2w (arm64) and pi-zero-w (arm-6) share an initPath, want distinct gosd-init binaries")
	}
}

// TestCompileForBoardsSurfacesAppCompileFailure confirms a compileApp failure
// is reported with the failing package and board, and stops before wasting a
// compileInit call.
func TestCompileForBoardsSurfacesAppCompileFailure(t *testing.T) {
	compileApp := func(_, _, _ string, _ boards.Arch) error { return errors.New("boom") }
	initCalls := 0
	compileInit := func(_, _ string, _ boards.Arch) error {
		initCalls++
		return nil
	}

	_, err := compileForBoards([]boards.Board{pizero2w.New()}, t.TempDir(), "./pkg", "", compileApp, compileInit)
	if err == nil {
		t.Fatal("compileForBoards succeeded despite a failing compileApp, want an error")
	}
	if initCalls != 0 {
		t.Errorf("compileInit was called after compileApp failed, want it skipped")
	}
}

// TestCompileForBoardsWritesDistinctAppPathsPerBoard guards against app
// binaries from different boards colliding on disk: even two boards sharing
// an arch must each get their own app path inside tempDir.
func TestCompileForBoardsWritesDistinctAppPathsPerBoard(t *testing.T) {
	c := &countingCompiler{}
	tempDir := t.TempDir()
	selected := []boards.Board{pizero2w.New(), radxazero3e.New(), pizerow.New()}

	binaries, err := compileForBoards(selected, tempDir, "./pkg", "", c.compileApp, c.compileInit)
	if err != nil {
		t.Fatalf("compileForBoards: %v", err)
	}

	seen := make(map[string]bool, len(selected))
	for _, b := range selected {
		p := binaries[b.Name()].appPath
		if seen[p] {
			t.Errorf("board %q's appPath %q collides with another board's", b.Name(), p)
		}
		seen[p] = true
		if filepath.Dir(p) != tempDir {
			t.Errorf("board %q's appPath %q is not inside tempDir %q", b.Name(), p, tempDir)
		}
	}
}
