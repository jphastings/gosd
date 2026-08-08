package build

import (
	"debug/elf"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jphastings/gosd/internal/boards"
)

// TestCrossCompileTsfunnelUsesLocalCheckoutByDefault mirrors
// TestCrossCompileGosdInitUsesLocalCheckoutByDefault for the shim: this test
// compiles real tailscale.com code for arm64 (bean gosd-kzd3 notes the
// module/build cache makes repeat runs tolerable).
func TestCrossCompileTsfunnelUsesLocalCheckoutByDefault(t *testing.T) {
	out := filepath.Join(t.TempDir(), "gosd-tsfunnel")

	if err := CrossCompileTsfunnel(out, "", arm64); err != nil {
		t.Fatalf("CrossCompileTsfunnel: %v", err)
	}

	f, err := elf.Open(out)
	if err != nil {
		t.Fatalf("output is not a valid ELF binary: %v", err)
	}
	defer func() { _ = f.Close() }()

	if f.Class != elf.ELFCLASS64 {
		t.Errorf("Class = %v, want %v (64-bit)", f.Class, elf.ELFCLASS64)
	}
	if f.Machine != elf.EM_AARCH64 {
		t.Errorf("Machine = %v, want %v (arm64)", f.Machine, elf.EM_AARCH64)
	}
}

// TestCrossCompileTsfunnelProducesStaticARMv6Binary proves the epic's "ALL
// boards" claim (Funnel facts: GOARM=6 self-compile covers pi-zero-w) with a
// real 32-bit build of the shim, not just the app/gosd-init binaries.
func TestCrossCompileTsfunnelProducesStaticARMv6Binary(t *testing.T) {
	out := filepath.Join(t.TempDir(), "gosd-tsfunnel")

	if err := CrossCompileTsfunnel(out, "", boards.Arch{GOARCH: "arm", GOARM: "6"}); err != nil {
		t.Fatalf("CrossCompileTsfunnel: %v", err)
	}

	f, err := elf.Open(out)
	if err != nil {
		t.Fatalf("output is not a valid ELF binary: %v", err)
	}
	defer func() { _ = f.Close() }()

	if f.Class != elf.ELFCLASS32 {
		t.Errorf("Class = %v, want %v (32-bit)", f.Class, elf.ELFCLASS32)
	}
	if f.Machine != elf.EM_ARM {
		t.Errorf("Machine = %v, want %v (arm)", f.Machine, elf.EM_ARM)
	}
}

// TestCrossCompileTsfunnelIsStripped confirms -ldflags="-s -w" (epic decision
// 2) actually reaches the shim's build: no symbol table (-s) and no DWARF
// debug info (-w), unlike gosd-init which is deliberately never stripped.
func TestCrossCompileTsfunnelIsStripped(t *testing.T) {
	out := filepath.Join(t.TempDir(), "gosd-tsfunnel")

	if err := CrossCompileTsfunnel(out, "", arm64); err != nil {
		t.Fatalf("CrossCompileTsfunnel: %v", err)
	}

	f, err := elf.Open(out)
	if err != nil {
		t.Fatalf("output is not a valid ELF binary: %v", err)
	}
	defer func() { _ = f.Close() }()

	if sec := f.Section(".symtab"); sec != nil {
		t.Error("output has a .symtab section, want it stripped by -ldflags=\"-s\"")
	}
	if sec := f.Section(".debug_info"); sec != nil {
		t.Error("output has a .debug_info section, want DWARF stripped by -ldflags=\"-w\"")
	}
}

// TestCrossCompileTsfunnelOverrideDirWins confirms the sibling-derivation
// documented on tsfunnelSrcDirName: passing gosd-init's own leaf directory
// as the --gosd-init-src override (exactly what a developer would set) must
// still locate and build the real cmd/gosd-tsfunnel sibling.
func TestCrossCompileTsfunnelOverrideDirWins(t *testing.T) {
	out := filepath.Join(t.TempDir(), "gosd-tsfunnel")

	if err := CrossCompileTsfunnel(out, "../../cmd/gosd-init", arm64); err != nil {
		t.Fatalf("CrossCompileTsfunnel with override: %v", err)
	}
	if info, err := os.Stat(out); err != nil || info.Size() == 0 {
		t.Errorf("expected a non-empty binary at %s", out)
	}
}

// TestCrossCompileTsfunnelRejectsMissingOverrideDir confirms a missing
// sibling directory is reported actionably, naming the same
// --gosd-init-src flag a developer would need to fix.
func TestCrossCompileTsfunnelRejectsMissingOverrideDir(t *testing.T) {
	err := CrossCompileTsfunnel(filepath.Join(t.TempDir(), "out"), filepath.Join(t.TempDir(), "does-not-exist", "cmd", "gosd-init"), arm64)
	if err == nil {
		t.Fatal("CrossCompileTsfunnel succeeded with a missing --gosd-init-src directory, want an error")
	}
	if !strings.Contains(err.Error(), "--gosd-init-src") {
		t.Errorf("error = %q, want it to mention --gosd-init-src", err)
	}
}

// TestTsfunnelOmitTagsExcludesRequiredFeaturesAndIncludesSSH pins the epic's
// tag-set decision (gosd-65uy decision 2) without paying for a real compile:
// netstack/serve/acme/bakedroots must be absent (the shim needs them), and
// ts_omit_ssh must be present (the "no interactive surface" compliance
// argument).
func TestTsfunnelOmitTagsExcludesRequiredFeaturesAndIncludesSSH(t *testing.T) {
	for _, required := range []string{"ts_omit_netstack", "ts_omit_serve", "ts_omit_acme", "ts_omit_bakedroots"} {
		if strings.Contains(tsfunnelOmitTags, required) {
			t.Errorf("tsfunnelOmitTags unexpectedly contains %q; this feature is required by the shim", required)
		}
	}
	if !strings.Contains(tsfunnelOmitTags, "ts_omit_ssh") {
		t.Error("tsfunnelOmitTags does not contain ts_omit_ssh, want Tailscale SSH compiled out entirely")
	}
}
