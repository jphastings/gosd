package repocheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jphastings/gosd/internal/boards"
	"github.com/jphastings/gosd/internal/boardset"
	"github.com/jphastings/gosd/internal/cacerts"
	"github.com/jphastings/gosd/internal/cloudflaredpin"
	"github.com/jphastings/gosd/internal/kernelspec"
)

// boardsRepoRoot is the checkout root relative to this package's directory.
// Each check file names its own rather than sharing one, so files landing
// here in parallel can't collide on a package-level identifier.
const boardsRepoRoot = "../.."

const (
	buildBoardsDir  = "build/boards"
	fakeArtifactDir = "cmd/gosd/testdata/fake-artifacts"
	compatibilityMD = "COMPATIBILITY.md"
	bringUpHeading  = "## Bring-up status"
)

// TestBoardRegistryIsPopulated is what makes the rest of this file mean
// anything: every other check iterates the registry, so an empty registry
// passes them all vacuously. It fails here first, naming the cause.
func TestBoardRegistryIsPopulated(t *testing.T) {
	if len(boardset.Registered()) == 0 {
		t.Fatal("boardset.Registered() is empty: internal/boardset's init() must register every board — " +
			"restore the register(...) calls there, or the rest of internal/repocheck's board checks pass vacuously")
	}
	if len(boards.All()) == 0 {
		t.Fatal("boards.All() is empty although boards are registered: every board in internal/boardset's " +
			"init() went through registerInternal — use register() for boards meant to be public")
	}
}

// TestKernelspecCoversTheRegisteredFleet holds both directions of "every
// board builds a kernel": gosd build-kernel --board <id> resolves <id>
// through internal/boards before looking up its spec, so an entry with no
// registered board is unreachable and a board with no entry is unbuildable.
func TestKernelspecCoversTheRegisteredFleet(t *testing.T) {
	for _, b := range boardset.Registered() {
		if _, ok := kernelspec.Get(b.Name()); !ok {
			t.Errorf("board %q has no kernelspec entry: add a %q key to the specs map in "+
				"internal/kernelspec/kernelspec.go, or gosd build-kernel --board %s has nothing to build",
				b.Name(), b.Name(), b.Name())
		}
	}

	registered := registeredIDs()
	for _, id := range kernelspec.BoardIDs() {
		if !registered[id] {
			t.Errorf("kernelspec entry %q has no registered board: register it in "+
				"internal/boardset/boardset.go (registerInternal is enough), or delete the %q key from the "+
				"specs map in internal/kernelspec/kernelspec.go — gosd build-kernel --board %s fails with "+
				"\"unknown board\" until the board exists",
				id, id, id)
		}
	}
}

// TestBuildBoardsDirsCoverTheRegisteredFleet keeps build/boards/<id>/ — the
// kernel fragment, patches and manifest a board's artifacts are built from —
// in step with the registry in both directions.
func TestBuildBoardsDirsCoverTheRegisteredFleet(t *testing.T) {
	for _, b := range boardset.Registered() {
		info, err := os.Stat(filepath.Join(boardsRepoRoot, buildBoardsDir, b.Name()))
		switch {
		case err != nil:
			t.Errorf("board %q has no %s/%s/ directory: create it with the board's kernel.fragment and "+
				"manifest.json (see an existing board's for the shape)", b.Name(), buildBoardsDir, b.Name())
		case !info.IsDir():
			t.Errorf("%s/%s is a file, not a directory: a board's build inputs live in a directory of that "+
				"name", buildBoardsDir, b.Name())
		}
	}

	entries, err := os.ReadDir(filepath.Join(boardsRepoRoot, buildBoardsDir))
	if err != nil {
		t.Fatalf("reading %s: %v", buildBoardsDir, err)
	}
	registered := registeredIDs()
	for _, e := range entries {
		if !registered[e.Name()] {
			t.Errorf("%s/%s has no registered board: register %q in internal/boardset/boardset.go "+
				"(registerInternal is enough), or delete %s/%s",
				buildBoardsDir, e.Name(), e.Name(), buildBoardsDir, e.Name())
		}
	}
}

// TestFakeArtifactsCoverEveryPublicBoard is the mechanical form of the
// activation rule PR #205 lost time to. cmd/gosd's network-tripwire
// integration tests build the default all-boards set against
// testdata/fake-artifacts, and ResolveArtifacts falls back to a real fetch
// for anything missing there — which a warm artifact cache in the developer's
// own HOME can satisfy, so only CI sees the tripwire fire. A URL-bearing ref
// is no exemption: the URL path is exactly the fetch being prevented.
//
// It iterates boards.All() rather than the whole fleet, and that is the only
// qemu-virt exemption there is: flipping a board from registerInternal to
// register in internal/boardset starts demanding fixtures here by itself.
func TestFakeArtifactsCoverEveryPublicBoard(t *testing.T) {
	dir := filepath.Join(boardsRepoRoot, fakeArtifactDir)
	wanted := map[string]bool{}

	for _, b := range boards.All() {
		for _, ref := range b.Artifacts() {
			if wanted[ref.Name] {
				continue // boards share artifacts; one report per file is enough
			}
			wanted[ref.Name] = true
			info, err := os.Stat(filepath.Join(dir, ref.Name))
			switch {
			case err != nil:
				t.Errorf("board %s needs artifact %q, which is missing from %s/: add a small placeholder "+
					"file of that name there, or cmd/gosd's integration tests fetch it from the network",
					b.Name(), ref.Name, fakeArtifactDir)
			case info.Size() == 0:
				t.Errorf("board %s's artifact fixture %s/%s is empty: write some placeholder bytes into it, "+
					"so a build that reads it produces something recognisable", b.Name(), fakeArtifactDir, ref.Name)
			}
		}
	}

	// The fixtures directory also carries the artifacts no board declares:
	// the ones gosd adds to every image, or per --ingress flag. Derived, so
	// a new feature artifact needs no edit here.
	wanted[cacerts.ArtifactName] = true
	for _, a := range cloudflaredpin.ByGOARCH {
		wanted[a.Name] = true
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", fakeArtifactDir, err)
	}
	for _, e := range entries {
		if !wanted[e.Name()] {
			t.Errorf("%s/%s is not named by any public board's Artifacts(), by internal/cacerts, or by "+
				"internal/cloudflaredpin: delete it, or fix the name to match the artifact it stands in for",
				fakeArtifactDir, e.Name())
		}
	}
}

// TestCompatibilityBringUpRows keeps COMPATIBILITY.md's bring-up table in
// step with the public fleet in both directions, by DisplayName().
//
// Only the "## Bring-up status" table is checked. The board × feature table
// below it deliberately abbreviates its column headers ("Pi Zero 2W" for
// Raspberry Pi Zero 2W, and similarly in 5 of its 7 columns) to keep the
// table readable, so its headers are not DisplayName() values and must not
// be asserted against them.
func TestCompatibilityBringUpRows(t *testing.T) {
	rows := bringUpTableBoards(t)

	for _, b := range boards.All() {
		if !rows[b.DisplayName()] {
			t.Errorf("%s's %q table has no row for %q (board %s): add a "+
				"\"| %s | <bring-up state> |\" row to it",
				compatibilityMD, bringUpHeading, b.DisplayName(), b.Name(), b.DisplayName())
		}
	}

	displayNames := map[string]bool{}
	for _, b := range boards.All() {
		displayNames[b.DisplayName()] = true
	}
	for name := range rows {
		if !displayNames[name] {
			t.Errorf("%s's %q table has a row for %q, which is no public board's DisplayName(): correct the "+
				"row's name to match the board's DisplayName(), or delete the row if the board is gone or "+
				"internal-only", compatibilityMD, bringUpHeading, name)
		}
	}
}

// bringUpTableBoards returns the first cell of every data row in
// COMPATIBILITY.md's bring-up table.
func bringUpTableBoards(t *testing.T) map[string]bool {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(boardsRepoRoot, compatibilityMD))
	if err != nil {
		t.Fatalf("reading %s: %v", compatibilityMD, err)
	}

	names := map[string]bool{}
	inTable := false
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == bringUpHeading:
			inTable = true
			continue
		case !inTable:
			continue
		case strings.HasPrefix(line, "## "):
			inTable = false
			continue
		case !strings.HasPrefix(line, "|"):
			continue
		}

		cell := strings.TrimSpace(strings.Split(strings.Trim(line, "|"), "|")[0])
		if cell == "" || cell == "Board" || strings.Trim(cell, "-: ") == "" {
			continue // header and separator rows
		}
		names[cell] = true
	}

	if len(names) == 0 {
		t.Fatalf("%s has no rows under %q: the table this check reads is gone or renamed", compatibilityMD, bringUpHeading)
	}
	return names
}

func registeredIDs() map[string]bool {
	ids := map[string]bool{}
	for _, b := range boardset.Registered() {
		ids[b.Name()] = true
	}
	return ids
}
