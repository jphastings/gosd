package kernelspec_test

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/jphastings/gosd/internal/boards"
	"github.com/jphastings/gosd/internal/boards/cubiea5e"
	"github.com/jphastings/gosd/internal/boards/nanopizero2"
	"github.com/jphastings/gosd/internal/boards/pi3b"
	"github.com/jphastings/gosd/internal/boards/pizero2w"
	"github.com/jphastings/gosd/internal/boards/pizerow"
	"github.com/jphastings/gosd/internal/boards/qemuvirt"
	"github.com/jphastings/gosd/internal/boards/radxazero3e"
	"github.com/jphastings/gosd/internal/boards/rock4se"
	"github.com/jphastings/gosd/internal/kernelspec"
)

var allBoardIDs = []string{"pi-zero-2w", "pi-zero-w", "pi-3b", "radxa-zero-3e", "nanopi-zero2", "rock-4se", "cubie-a5e", "qemu-virt"}

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
// not (yet) tracked by the board's internal/boards.Board.Artifacts(). Empty
// now that bean gosd-f59k wired pi-zero-2w's DTB into its Board.Artifacts()/
// BootFiles() - the one board that had the gap (pi-zero-w already tracked
// its own). Kept as an escape hatch so a deliberately-still-pending future
// gap doesn't need to touch this test's structure to be recorded.
var dtbExemptFromArtifacts = map[string]bool{}

// TestKernelSpecOutputsMatchBoardArtifacts is the drift guard the bean
// calls for: every filename a KernelSpec says the kernel build produces
// must be one of the board's declared artifacts, so the two single sources
// of truth (KernelSpec here, Board.Artifacts() in internal/boards) cannot
// silently diverge.
func TestKernelSpecOutputsMatchBoardArtifacts(t *testing.T) {
	boardsByID := map[string]boards.Board{
		"pi-zero-2w":    pizero2w.New(),
		"pi-zero-w":     pizerow.New(),
		"pi-3b":         pi3b.New(),
		"radxa-zero-3e": radxazero3e.New(),
		"nanopi-zero2":  nanopizero2.New(),
		"rock-4se":      rock4se.New(),
		"cubie-a5e":     cubiea5e.New(),
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

			// AdditionalDTBs get no exemption map: a board only lists one
			// when it means to ship it (pi-3b's 3B+ blob, bean gosd-oq0z).
			for _, dtb := range spec.AdditionalDTBs {
				if !artifactNames[dtb.Filename] {
					t.Errorf("AdditionalDTBs filename %q is not in %s's Artifacts()", dtb.Filename, id)
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

// TestDTSPatchesOnlyOnExpectedBoards guards against DTS patches silently
// appearing on (or vanishing from) a board: the Rockchip-family boards carry
// peripheral-enablement patches, and pi-zero-w carries the peripheral
// dma-ranges window its mainline-style DT lacks (bean gosd-1ey5). cubie-a5e
// joins the fleet's non-Rockchip side with NO patches - not an oversight,
// but a locked decision (bean gosd-axtv): header I2C/SPI enablement is
// deferred to a post-bring-up follow-up, and at the pinned kernel tag the
// dtsi has no SPI node at all for a patch to target. Every other board must
// build its device tree unpatched.
func TestDTSPatchesOnlyOnExpectedBoards(t *testing.T) {
	wantPatched := map[string]bool{
		"radxa-zero-3e": true,
		"nanopi-zero2":  true,
		"rock-4se":      true,
		"pi-zero-w":     true,
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

// TestAdditionalDTBsOnlyOnExpectedBoards guards against extra DTBs silently
// appearing on (or vanishing from) a board: only pi-3b ships a second blob
// (the 3B+'s, so one image covers the whole 3B family - bean gosd-oq0z).
// Every other board builds at most its single primary DTB.
func TestAdditionalDTBsOnlyOnExpectedBoards(t *testing.T) {
	wantAdditional := map[string][]string{
		"pi-3b": {"bcm2710-rpi-3-b-plus.dtb"},
	}

	for _, id := range allBoardIDs {
		spec, ok := kernelspec.Get(id)
		if !ok {
			t.Fatalf("Get(%q) not found", id)
		}
		var got []string
		for _, dtb := range spec.AdditionalDTBs {
			got = append(got, dtb.Filename)
		}
		if !equalStrings(got, wantAdditional[id]) {
			t.Errorf("%s: AdditionalDTBs filenames = %v, want %v", id, got, wantAdditional[id])
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
		"pi-3b":      "../../build/boards/pi-3b/kernel.fragment",
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
