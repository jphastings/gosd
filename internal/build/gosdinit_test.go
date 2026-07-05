package build

import (
	"debug/elf"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCrossCompileGosdInitUsesLocalCheckoutByDefault(t *testing.T) {
	out := filepath.Join(t.TempDir(), "gosd-init")

	if err := CrossCompileGosdInit(out, ""); err != nil {
		t.Fatalf("CrossCompileGosdInit: %v", err)
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

func TestCrossCompileGosdInitOverrideDirWins(t *testing.T) {
	out := filepath.Join(t.TempDir(), "gosd-init")

	// ../../cmd/gosd-init is gosd-init's real source, reached directly rather
	// than through detection - proving --gosd-init-src short-circuits the
	// ladder entirely.
	if err := CrossCompileGosdInit(out, "../../cmd/gosd-init"); err != nil {
		t.Fatalf("CrossCompileGosdInit with override: %v", err)
	}
	if info, err := os.Stat(out); err != nil || info.Size() == 0 {
		t.Errorf("expected a non-empty binary at %s", out)
	}
}

func TestCrossCompileGosdInitRejectsMissingOverrideDir(t *testing.T) {
	err := CrossCompileGosdInit(filepath.Join(t.TempDir(), "out"), filepath.Join(t.TempDir(), "does-not-exist"))
	if err == nil {
		t.Fatal("CrossCompileGosdInit succeeded with a missing --gosd-init-src directory, want an error")
	}
	if !strings.Contains(err.Error(), "--gosd-init-src") {
		t.Errorf("error = %q, want it to mention --gosd-init-src", err)
	}
}

func TestDevCheckoutDirFindsRepoRoot(t *testing.T) {
	dir, ok := devCheckoutDir()
	if !ok {
		t.Fatal("devCheckoutDir() = not found, want the gosd checkout running these tests")
	}
	if _, err := os.Stat(filepath.Join(dir, "cmd", "gosd-init")); err != nil {
		t.Errorf("devCheckoutDir() = %s, but it has no cmd/gosd-init: %v", dir, err)
	}
}

func TestModuleRootForModuleRejectsUnrelatedDir(t *testing.T) {
	if _, ok := moduleRootForModule(t.TempDir()); ok {
		t.Error("moduleRootForModule(unrelated dir) = found, want not found")
	}
}

// TestModuleCacheDirRejectsDevelVersion documents rung 2's most important
// failure mode: `go test` (like any other unreleased build) always reports
// its own module version as "(devel)", so this exercises the real
// actionable error a developer gets when gosd itself was built the same
// way, outside of a checkout devCheckoutDir can find.
func TestModuleCacheDirRejectsDevelVersion(t *testing.T) {
	_, err := moduleCacheDir()
	if err == nil {
		t.Fatal("moduleCacheDir() succeeded for a (devel) build, want an actionable error")
	}
	if !strings.Contains(err.Error(), "--gosd-init-src") {
		t.Errorf("error = %q, want it to mention the --gosd-init-src escape hatch", err)
	}
}
