package kernelspec_test

import (
	"bytes"
	"os"
	"path/filepath"
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
	"github.com/jphastings/gosd/internal/kernelspec"
)

var allBoardIDs = []string{"pi-zero-2w", "pi-zero-w", "radxa-zero-3e", "nanopi-zero2", "qemu-virt"}

func TestBoardIDsListsExactlyTheFiveKernelBuildingBoards(t *testing.T) {
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
// bean gosd-di6v rather than silently worked around. pi-zero-2w's build.sh
// produces bcm2710-rpi-zero-2-w.dtb, but internal/boards/pizero2w never
// asks for a DTB artifact (unlike pi-zero-w) - the GPU firmware's own
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

// parseBashArray extracts a bash array literal's elements, e.g.
// `required_y=(\n  CONFIG_FOO\n  CONFIG_BAR\n)`. It's a narrow parser: it
// assumes (true of every docker-build.sh this reads) the array body
// contains no nested parentheses and one bare identifier per line.
func parseBashArray(src []byte, name string) []string {
	marker := []byte(name + "=(")
	idx := bytes.Index(src, marker)
	if idx == -1 {
		return nil
	}
	rest := src[idx+len(marker):]
	end := bytes.IndexByte(rest, ')')
	if end == -1 {
		return nil
	}
	var items []string
	for _, line := range strings.Split(string(rest[:end]), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		items = append(items, line)
	}
	return items
}

// TestRockchipRequiredYMatchesScript compares each Rockchip-family board's
// KernelSpec.RequiredY (and, for qemu-virt, ForbiddenY) against the literal
// required_y/forbidden_y bash arrays in that board's docker-build.sh, so the
// duplication this bean's design doc calls out (see kernelspec.go's package
// doc comment) can't silently drift before gosd-07fl removes it.
func TestRockchipRequiredYMatchesScript(t *testing.T) {
	cases := []struct {
		boardID string
		script  string
	}{
		{"radxa-zero-3e", "../../build/boards/radxa-zero-3e/kernel/docker-build.sh"},
		{"nanopi-zero2", "../../build/boards/nanopi-zero2/kernel/docker-build.sh"},
		{"qemu-virt", "../../build/boards/qemu-virt/kernel/docker-build.sh"},
	}

	for _, c := range cases {
		t.Run(c.boardID, func(t *testing.T) {
			spec, ok := kernelspec.Get(c.boardID)
			if !ok {
				t.Fatalf("Get(%q) not found", c.boardID)
			}
			src, err := os.ReadFile(filepath.FromSlash(c.script))
			if err != nil {
				t.Fatalf("reading %s: %v", c.script, err)
			}

			wantRequired := parseBashArray(src, "required_y")
			if len(wantRequired) == 0 {
				t.Fatalf("parsed no required_y entries from %s; parser or script likely changed shape", c.script)
			}
			if !equalStrings(spec.RequiredY, wantRequired) {
				t.Errorf("RequiredY = %v, want %v (parsed from %s)", spec.RequiredY, wantRequired, c.script)
			}

			wantForbidden := parseBashArray(src, "forbidden_y")
			if !equalStrings(spec.ForbiddenY, wantForbidden) {
				t.Errorf("ForbiddenY = %v, want %v (parsed from %s)", spec.ForbiddenY, wantForbidden, c.script)
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
