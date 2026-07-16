package kernelspec_test

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/jphastings/gosd/internal/boards"
	"github.com/jphastings/gosd/internal/boards/nanopizero2"
	"github.com/jphastings/gosd/internal/boards/pizero2w"
	"github.com/jphastings/gosd/internal/boards/pizerow"
	"github.com/jphastings/gosd/internal/boards/qemuvirt"
	"github.com/jphastings/gosd/internal/boards/radxazero3e"
	"github.com/jphastings/gosd/internal/boards/rock4se"
	"github.com/jphastings/gosd/internal/kernelspec"
)

var allBoardIDs = []string{"pi-zero-2w", "pi-zero-w", "radxa-zero-3e", "nanopi-zero2", "rock-4se", "qemu-virt"}

func TestBoardIDsListsExactlyTheKernelBuildingBoards(t *testing.T) {
	got := kernelspec.BoardIDs()
	want := append([]string(nil), allBoardIDs...)
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("BoardIDs() = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("BoardIDs()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSpecResolutionIsComplete(t *testing.T) {
	for _, id := range allBoardIDs {
		t.Run(id, func(t *testing.T) {
			spec, ok := kernelspec.Get(id)
			if !ok {
				t.Fatalf("Get(%q) not found", id)
			}
			if spec.Source.Repo == "" {
				t.Error("Source.Repo is empty")
			}
			if spec.Source.Ref == "" {
				t.Error("Source.Ref is empty")
			}
			if spec.Defconfig == "" {
				t.Error("Defconfig is empty")
			}
			if spec.Toolchain.KernelArch == "" {
				t.Error("Toolchain.KernelArch is empty")
			}
			if spec.Toolchain.CrossCompile == "" {
				t.Error("Toolchain.CrossCompile is empty")
			}
			if spec.KernelMakeTarget == "" {
				t.Error("KernelMakeTarget is empty")
			}
			if spec.KernelSourcePath == "" {
				t.Error("KernelSourcePath is empty")
			}
			if spec.KernelFilename == "" {
				t.Error("KernelFilename is empty")
			}
			if len(spec.RequiredY) == 0 {
				t.Error("RequiredY is empty")
			}
			if !spec.ModulesDisabled {
				t.Error("ModulesDisabled = false, want true for every current board")
			}
		})
	}
}

func TestGetUnknownBoardReturnsNotFound(t *testing.T) {
	if _, ok := kernelspec.Get("not-a-board"); ok {
		t.Error("Get(\"not-a-board\") returned ok = true, want false")
	}
}

// dtbExemptFromArtifacts documents boards whose KernelSpec.DTB.Filename is
// not (yet) tracked by the board's internal/boards.Board.Artifacts(): a
// pre-existing gap discovered while writing this drift test, recorded in
// bean gosd-di6v rather than silently worked around. pi-zero-2w's kernel
// build produces bcm2710-rpi-zero-2-w.dtb, but internal/boards/pizero2w
// never asks for a DTB artifact (unlike pi-zero-w) - the GPU firmware's own
// fallback device tree is used instead. Fixing that wiring is out of scope
// for this bean; see the bean body for the follow-up note.
var dtbExemptFromArtifacts = map[string]bool{
	"pi-zero-2w": true,
}

// TestKernelSpecOutputsMatchBoardArtifacts is the drift guard the bean
// calls for: every filename a KernelSpec says the kernel build produces
// must be one of the board's declared artifacts, so the two single sources
// of truth (KernelSpec here, Board.Artifacts() in internal/boards) cannot
// silently diverge.
func TestKernelSpecOutputsMatchBoardArtifacts(t *testing.T) {
	boardsByID := map[string]boards.Board{
		"pi-zero-2w":    pizero2w.New(),
		"pi-zero-w":     pizerow.New(),
		"radxa-zero-3e": radxazero3e.New(),
		"nanopi-zero2":  nanopizero2.New(),
		"rock-4se":      rock4se.New(),
		"qemu-virt":     qemuvirt.New(),
	}

	for _, id := range allBoardIDs {
		t.Run(id, func(t *testing.T) {
			spec, ok := kernelspec.Get(id)
			if !ok {
				t.Fatalf("Get(%q) not found", id)
			}
			b, ok := boardsByID[id]
			if !ok {
				t.Fatalf("no internal/boards.Board wired up in this test for %q", id)
			}

			artifactNames := make(map[string]bool)
			for _, ref := range b.Artifacts() {
				artifactNames[ref.Name] = true
			}

			if !artifactNames[spec.KernelFilename] {
				t.Errorf("KernelFilename %q is not in %s's Artifacts()", spec.KernelFilename, id)
			}

			if spec.DTB != nil && spec.DTB.Filename != "" && !dtbExemptFromArtifacts[id] {
				if !artifactNames[spec.DTB.Filename] {
					t.Errorf("DTB.Filename %q is not in %s's Artifacts()", spec.DTB.Filename, id)
				}
			}
		})
	}
}

func TestEmbeddedConfigFragmentsAreNonEmpty(t *testing.T) {
	for _, id := range allBoardIDs {
		spec, ok := kernelspec.Get(id)
		if !ok {
			t.Fatalf("Get(%q) not found", id)
		}
		if len(spec.ConfigFragment) == 0 {
			t.Errorf("%s: ConfigFragment is empty", id)
		}
	}
}

func TestDTSPatchesOnlyOnRockchipBoards(t *testing.T) {
	wantPatched := map[string]bool{
		"radxa-zero-3e": true,
		"nanopi-zero2":  true,
		"rock-4se":      true,
	}

	for _, id := range allBoardIDs {
		spec, ok := kernelspec.Get(id)
		if !ok {
			t.Fatalf("Get(%q) not found", id)
		}

		if wantPatched[id] {
			if len(spec.DTSPatches) == 0 {
				t.Errorf("%s: want DTS patches, got none", id)
			}
			for _, p := range spec.DTSPatches {
				if len(p.Content) == 0 {
					t.Errorf("%s: patch %q has empty content", id, p.Name)
				}
			}
		} else if len(spec.DTSPatches) != 0 {
			t.Errorf("%s: want no DTS patches, got %d", id, len(spec.DTSPatches))
		}
	}
}

// configYLine mirrors the pattern kernelspec.go uses to derive the Pi
// boards' RequiredY from their kernel.fragment - reimplemented here,
// against the on-disk fragment file rather than the embedded copy, so this
// test can catch drift between the fragment and the spec's RequiredY
// without importing kernelspec's unexported helper.
var configYLine = regexp.MustCompile(`^CONFIG_[A-Z0-9_]+=y$`)

func requiredYFromFragmentFile(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	var required []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimRight(line, "\r")
		if configYLine.MatchString(line) {
			required = append(required, line)
		}
	}
	return required
}

func TestPiRequiredYIsDerivedFromFragment(t *testing.T) {
	cases := map[string]string{
		"pi-zero-2w": "../../build/boards/pi-zero-2w/kernel.fragment",
		"pi-zero-w":  "../../build/boards/pi-zero-w/kernel.fragment",
	}

	for id, fragmentPath := range cases {
		t.Run(id, func(t *testing.T) {
			spec, ok := kernelspec.Get(id)
			if !ok {
				t.Fatalf("Get(%q) not found", id)
			}
			want := requiredYFromFragmentFile(t, fragmentPath)
			if !equalStrings(spec.RequiredY, want) {
				t.Errorf("RequiredY = %v, want %v (derived from %s)", spec.RequiredY, want, fragmentPath)
			}
		})
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
