package kernelspec_test

import (
	"os"
	"strings"
	"testing"

	"github.com/jphastings/gosd/internal/kernelspec"
)

// kernelConfigSnapshotPath is where each board's committed "full .config a
// real build produces" reference snapshot lives (bean gosd-ilv8; see each
// board's README.md). There's no on-disk naming convention to derive this
// from - the Pi boards keep kernel.config flat under their board directory,
// every other board nests it under kernel/ - so every kernel-building board
// needs an explicit entry; TestKernelConfigSnapshotCoversEveryBoard enforces
// that.
var kernelConfigSnapshotPath = map[string]string{
	"pi-zero-2w":    "../../build/boards/pi-zero-2w/kernel.config",
	"pi-zero-w":     "../../build/boards/pi-zero-w/kernel.config",
	"pi-3b":         "../../build/boards/pi-3b/kernel.config",
	"radxa-zero-3e": "../../build/boards/radxa-zero-3e/kernel/kernel.config",
	"nanopi-zero2":  "../../build/boards/nanopi-zero2/kernel/kernel.config",
	"rock-4se":      "../../build/boards/rock-4se/kernel/kernel.config",
	"cubie-a5e":     "../../build/boards/cubie-a5e/kernel/kernel.config",
	"qemu-virt":     "../../build/boards/qemu-virt/kernel/kernel.config",
}

func TestKernelConfigSnapshotCoversEveryBoard(t *testing.T) {
	for _, id := range allBoardIDs {
		if _, ok := kernelConfigSnapshotPath[id]; !ok {
			t.Errorf("board %q has no kernelConfigSnapshotPath entry; add the path to its committed kernel.config", id)
		}
	}
}

// knownKernelConfigSnapshotDrift is a ratchet, not a suppression list: each
// entry names a CONFIG symbol where, as of bean gosd-ilv8, the board's
// COMMITTED kernel.config snapshot already disagrees with kernelspec's
// CURRENT RequiredY/ForbiddenY/ModulesDisabled assertions - i.e. the
// snapshot is stale and hasn't been regenerated since the assertion
// changed. Regenerating a snapshot needs a real (20-75 minute, Docker-backed)
// `gosd build-kernel` run, so this list exists purely so
// TestKernelConfigSnapshotMatchesAssertions can gate on *new* drift starting
// now without being blocked on rebuilding every board's kernel first; it is
// not a statement that the drift is fine to leave. Follow-up: bean
// gosd-dcov tracks regenerating these snapshots for real.
//
// This is the exact failure mode CLAUDE.md's "Board work" section already
// names as having happened once (bean gosd-95yu believed "the Pi family has
// no ext4" off a stale snapshot like these, for months after the fragment
// actually gained it) - the entries below are that same class of staleness,
// caught mechanically this time instead of by accident. The kernel.config
// snapshot is documented as advisory, never authoritative (see each board's
// README.md and kernelspec's package doc): a fragment/RequiredY change is
// never blocked on updating it, and this test does not change that - it
// only makes the resulting staleness visible instead of silent.
//
// An entry that stops reproducing (the snapshot was regenerated, or the
// assertion was relaxed) must be deleted: the test fails either way, so
// this list can only shrink, never silently grow.
var knownKernelConfigSnapshotDrift = map[string][]string{
	// Every Pi board's kernel.fragment requires CONFIG_EXT4_FS=y (disk's
	// ext4 default for attached USB mass storage, bean gosd-19kw), but none
	// of the three committed snapshots have been regenerated since.
	"pi-zero-2w": {"CONFIG_EXT4_FS"},
	"pi-zero-w":  {"CONFIG_EXT4_FS", "CONFIG_MMC_SDHCI_IPROC"},
	"pi-3b":      {"CONFIG_EXT4_FS"},
	// Both Rockchip boards' fragments require CONFIG_EXFAT_FS=y (the `disk`
	// package's exFAT support, same rationale as every other board's
	// fragment), but neither committed snapshot has been regenerated since.
	"radxa-zero-3e": {"CONFIG_EXFAT_FS"},
	"nanopi-zero2":  {"CONFIG_EXFAT_FS"},
	// qemu-virt's fragment explicitly forbids both symbols (see its
	// ForbiddenY comments: the legacy mass-storage gadget auto-binds the
	// UDC with no backing file, bean gosd-sz6p; nothing in gosd formats or
	// mounts btrfs, bean gosd-10fn) but the committed snapshot predates both
	// cuts and still has them =y.
	"qemu-virt": {"CONFIG_USB_MASS_STORAGE", "CONFIG_BTRFS_FS"},
}

// trimConfigSuffix strips a trailing "=y"/"=m", mirroring
// internal/kernelbuild's normalizeSymbols: RequiredY/ForbiddenY entries
// store either the bare symbol (every non-Pi board) or the full fragment
// line (the Pi boards, derived verbatim - see requiredYFromFragment).
func trimConfigSuffix(opt string) string {
	opt = strings.TrimSuffix(opt, "=y")
	opt = strings.TrimSuffix(opt, "=m")
	return opt
}

// TestKernelConfigSnapshotMatchesAssertions is the drift guard bean
// gosd-ilv8 calls for: it never treats the committed kernel.config as
// authoritative (kernelspec's RequiredY/ForbiddenY/ModulesDisabled - in turn
// derived from or checked against ConfigFragment - stay the assertion; see
// TestPiRequiredYIsDerivedFromFragment and
// TestNonPiRequiredYForbiddenYAppearInOwnFragment), only mechanically
// checkable rather than silently stale: every RequiredY symbol must appear
// as "=y" in the snapshot, every ForbiddenY symbol must not appear as "=y"
// or "=m", and CONFIG_MODULES must read "is not set" when ModulesDisabled.
// A board with no unexpected disagreement passes; a newly introduced one
// fails immediately instead of sitting silently in the repo the way the
// gosd-95yu incident did.
func TestKernelConfigSnapshotMatchesAssertions(t *testing.T) {
	for _, id := range allBoardIDs {
		t.Run(id, func(t *testing.T) {
			path, ok := kernelConfigSnapshotPath[id]
			if !ok {
				t.Fatalf("no kernelConfigSnapshotPath entry for %q", id)
			}
			spec, ok := kernelspec.Get(id)
			if !ok {
				t.Fatalf("Get(%q) not found", id)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("reading %s: %v", path, err)
			}
			lines := make(map[string]bool)
			for _, line := range strings.Split(string(data), "\n") {
				lines[strings.TrimRight(line, "\r")] = true
			}

			disagree := make(map[string]bool)
			for _, r := range spec.RequiredY {
				sym := trimConfigSuffix(r)
				if !lines[sym+"=y"] {
					disagree[sym] = true
				}
			}
			for _, f := range spec.ForbiddenY {
				sym := trimConfigSuffix(f)
				if lines[sym+"=y"] || lines[sym+"=m"] {
					disagree[sym] = true
				}
			}
			if spec.ModulesDisabled && !lines["# CONFIG_MODULES is not set"] {
				disagree["CONFIG_MODULES"] = true
			}

			known := make(map[string]bool)
			for _, sym := range knownKernelConfigSnapshotDrift[id] {
				known[sym] = true
			}

			for sym := range disagree {
				if !known[sym] {
					t.Errorf("NEW snapshot drift: %s disagrees with kernelspec's current assertions on %s (add it to knownKernelConfigSnapshotDrift and file/point at a bean, or regenerate the snapshot via `gosd build-kernel --board %s -o out/`)", path, sym, id)
				}
			}
			for sym := range known {
				if !disagree[sym] {
					t.Errorf("stale knownKernelConfigSnapshotDrift[%q] entry %q: %s no longer disagrees with it; remove the entry", id, sym, path)
				}
			}
		})
	}
}
