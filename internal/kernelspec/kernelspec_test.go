package kernelspec_test

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/jphastings/gosd/internal/boards"
	"github.com/jphastings/gosd/internal/boardset"
	"github.com/jphastings/gosd/internal/kernelspec"
)

// allBoardIDs is every board gosd registers, internal-only ones included:
// each one builds a kernel, so the board registry is the list. Deriving it
// rather than repeating it means adding a board can't leave this file
// silently testing the old fleet.
var allBoardIDs = boardIDsFromRegistry()

func boardIDsFromRegistry() []string {
	var ids []string
	for _, b := range boardset.Registered() {
		ids = append(ids, b.Name())
	}
	return ids
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
	for _, id := range allBoardIDs {
		t.Run(id, func(t *testing.T) {
			spec, ok := kernelspec.Get(id)
			if !ok {
				t.Fatalf("Get(%q) not found", id)
			}
			// allBoardIDs comes from the registry, so this always resolves.
			b, _ := boards.Find(id)

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

			// No exemptions: a board only lists an AdditionalDTB when it
			// means to ship it, and the pinned release must already carry
			// it (see docs/artifacts.md's tag-first, bump-second order).
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
// dma-ranges window its mainline-style DT lacks (bean gosd-1ey5). Every Pi
// board and every board with a status LED (all but qemu-virt) also carries
// a retain-state-shutdown/default-state patch on its status LED node (bean
// gosd-54j8 - see docs/status-led.md). cubie-a5e's header I2C/SPI enablement
// remains deferred to a post-bring-up follow-up (locked decision, bean
// gosd-axtv): at the pinned kernel tag the dtsi has no SPI node at all for
// such a patch to target, but it still carries the LED patch below. Only
// qemu-virt must build its device tree unpatched (it has no DTB of its own).
func TestDTSPatchesOnlyOnExpectedBoards(t *testing.T) {
	wantPatched := map[string]bool{
		"radxa-zero-3e": true,
		"nanopi-zero2":  true,
		"rock-4se":      true,
		"pi-zero-w":     true,
		"pi-zero-2w":    true,
		"pi-3b":         true,
		// Not peripheral enablement like the Rockchip boards: this one
		// adds a second, USB-gadget variant DTB (bean gosd-3io0), plus the
		// status-LED retain-state-shutdown patch every board with an LED
		// carries (bean gosd-54j8).
		"cubie-a5e": true,
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
// appearing on (or vanishing from) a board. Two ship a second blob, for
// opposite reasons: pi-3b carries the 3B+'s so one image covers the whole 3B
// family (bean gosd-oq0z), while cubie-a5e carries a variant of its OWN DTB
// that --usb-gadget selects instead of the stock one (bean gosd-3io0).
// Every other board builds at most its single primary DTB.
func TestAdditionalDTBsOnlyOnExpectedBoards(t *testing.T) {
	wantAdditional := map[string][]string{
		"pi-3b": {"bcm2710-rpi-3-b-plus.dtb"},
		// The gadget variant of the same board: identical except
		// ehci0/ohci0 are disabled so MUSB keeps the USB-C port's phy
		// (bean gosd-3io0). --usb-gadget picks which one ships.
		"cubie-a5e": {"sun55i-a527-cubie-a5e-gadget.dtb"},
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

// nonPiBoardsWithHandWrittenRequiredY lists every board whose
// RequiredY/ForbiddenY is a hand-maintained literal list rather than
// requiredYFromFragment-derived (see TestPiRequiredYIsDerivedFromFragment
// for the Pi boards - bean gosd-hufu). Unlike the Pi boards, a Rockchip/
// Allwinner/qemu-virt fragment routinely sets a symbol =y as a baseline
// default (inherited from `make defconfig`) without that symbol being part
// of GoSD's own requirement list, so this can't assert exact equality with
// the fragment's CONFIG_*=y lines the way the Pi test does. It can still
// catch the two concrete error classes the bean names: a RequiredY/
// ForbiddenY entry that's a typo'd CONFIG_ name, and a RequiredY entry for a
// config this board's own fragment never actually sets.
var nonPiBoardsWithHandWrittenRequiredY = []string{
	"radxa-zero-3e", "nanopi-zero2", "rock-4se", "cubie-a5e", "qemu-virt",
}

// TestNonPiRequiredYForbiddenYAppearInOwnFragment is the cross-check bean
// gosd-hufu calls for: every hand-written RequiredY entry must appear as a
// literal "CONFIG_FOO=y" line somewhere in that board's own ConfigFragment
// (device-tree patches never touch Kconfig, so every RequiredY/ForbiddenY
// symbol's source is the fragment - verified by inspection for every board
// this covers), and every hand-written ForbiddenY entry must NOT appear as
// a literal "CONFIG_FOO=y" line in it (a forbidden symbol the fragment
// itself turns on would be a self-contradictory spec). This doesn't derive
// the lists the way the Pi boards' RequiredY is derived - see the var doc
// above for why not - but it does mean a typo'd or dead assertion fails
// `go test ./...` instead of surviving unnoticed until a bench boot.
func TestNonPiRequiredYForbiddenYAppearInOwnFragment(t *testing.T) {
	for _, id := range nonPiBoardsWithHandWrittenRequiredY {
		t.Run(id, func(t *testing.T) {
			spec, ok := kernelspec.Get(id)
			if !ok {
				t.Fatalf("Get(%q) not found", id)
			}

			setY := make(map[string]bool)
			for _, line := range strings.Split(string(spec.ConfigFragment), "\n") {
				line = strings.TrimRight(line, "\r")
				if configYLine.MatchString(line) {
					setY[line] = true
				}
			}

			for _, want := range spec.RequiredY {
				sym := strings.TrimSuffix(strings.TrimSuffix(want, "=y"), "=m")
				if !setY[sym+"=y"] {
					t.Errorf("RequiredY entry %q does not appear as %q in %s's own ConfigFragment; typo, or an assertion for a config this board's fragment doesn't set", want, sym+"=y", id)
				}
			}

			for _, forbidden := range spec.ForbiddenY {
				sym := strings.TrimSuffix(strings.TrimSuffix(forbidden, "=y"), "=m")
				if setY[sym+"=y"] {
					t.Errorf("ForbiddenY entry %q contradicts its own fragment: %s's ConfigFragment sets %q", forbidden, id, sym+"=y")
				}
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
