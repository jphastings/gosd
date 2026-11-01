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
	"pi-cm4":        "../../build/boards/pi-cm4/kernel.config",
	"radxa-zero-3e": "../../build/boards/radxa-zero-3e/kernel/kernel.config",
	"nanopi-zero2":  "../../build/boards/nanopi-zero2/kernel/kernel.config",
	"rock-4se":      "../../build/boards/rock-4se/kernel/kernel.config",
	"cubie-a5e":     "../../build/boards/cubie-a5e/kernel/kernel.config",
	"qemu-virt":     "../../build/boards/qemu-virt/kernel/kernel.config",
	"turing-rk1":    "../../build/boards/turing-rk1/kernel/kernel.config",
}

func TestKernelConfigSnapshotCoversEveryBoard(t *testing.T) {
	for _, id := range allBoardIDs {
		if _, ok := kernelConfigSnapshotPath[id]; !ok {
			t.Errorf("board %q has no kernelConfigSnapshotPath entry; add the path to its committed kernel.config", id)
		}
	}
}

// knownKernelConfigSnapshotDrift is a ratchet, not a suppression list: each
// entry names a CONFIG symbol where a board's COMMITTED kernel.config
// snapshot disagrees with kernelspec's CURRENT RequiredY/ForbiddenY/
// ModulesDisabled assertions - i.e. the snapshot is stale and hasn't been
// regenerated since the assertion changed. Regenerating a snapshot needs a
// real (20-75 minute, Docker-backed) `gosd build-kernel` run - or a
// workflow_dispatch of build-artifacts.yml, whose per-board uploads carry
// kernel.config - so this list lets TestKernelConfigSnapshotMatchesAssertions
// gate on *new* drift without blocking on a rebuild; it is not a statement
// that the drift is fine to leave. Bean gosd-ilv8 seeded it with the drift
// that predated this test; bean gosd-d6z2 regenerated every snapshot from a
// real build and emptied it (closing gosd-dcov).
//
// This is the exact failure mode CLAUDE.md's "Board work" section already
// names as having happened once (bean gosd-95yu believed "the Pi family has
// no ext4" off a stale snapshot, for months after the fragment actually
// gained it) - caught mechanically now instead of by accident. The kernel.config
// snapshot is documented as advisory, never authoritative (see each board's
// README.md and kernelspec's package doc): a fragment/RequiredY change is
// never blocked on updating it, and this test does not change that - it
// only makes the resulting staleness visible instead of silent.
//
// An entry that stops reproducing (the snapshot was regenerated, or the
// assertion was relaxed) must be deleted: the test fails either way, so
// this list can only shrink, never silently grow.
var knownKernelConfigSnapshotDrift = map[string][]string{}

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
