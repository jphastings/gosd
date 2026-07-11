package kernelconfig_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jphastings/gosd/internal/kernelconfig"
)

func TestParseEmptyIsNoop(t *testing.T) {
	cfg, err := kernelconfig.Parse(nil)
	if err != nil {
		t.Fatalf("Parse(nil): %v", err)
	}
	if len(cfg.Kernel) != 0 {
		t.Errorf("Parse(nil).Kernel = %v, want empty", cfg.Kernel)
	}
}

func TestParseFragmentAndPatches(t *testing.T) {
	data := []byte(`
[kernel.radxa-zero-3e]
fragment = "fragments/dvb.config"
patches = ["patches/*.patch"]
`)
	cfg, err := kernelconfig.Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	board, ok := cfg.Kernel["radxa-zero-3e"]
	if !ok {
		t.Fatal("Parse did not produce a [kernel.radxa-zero-3e] entry")
	}
	if board.Fragment != "fragments/dvb.config" {
		t.Errorf("Fragment = %q, want fragments/dvb.config", board.Fragment)
	}
	if len(board.Patches) != 1 || board.Patches[0] != "patches/*.patch" {
		t.Errorf("Patches = %v, want [patches/*.patch]", board.Patches)
	}
}

func TestParseMalformedTOMLErrorsActionably(t *testing.T) {
	_, err := kernelconfig.Parse([]byte("this is not [ valid toml"))
	if err == nil {
		t.Fatal("Parse(malformed) succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "gosd-kernel.toml") {
		t.Errorf("error = %q, want it to name gosd-kernel.toml", err.Error())
	}
}

func TestOverlayNoMatchingSectionIsNoop(t *testing.T) {
	cfg, err := kernelconfig.Parse([]byte(`[kernel.radxa-zero-3e]
fragment = "dvb.config"
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	overlay, err := cfg.Overlay("pi-zero-2w", t.TempDir())
	if err != nil {
		t.Fatalf("Overlay: %v", err)
	}
	if overlay.ConfigFragment != nil || overlay.Patches != nil {
		t.Errorf("Overlay for a board with no section = %+v, want the zero value", overlay)
	}
}

func TestOverlayReadsFragmentAndPatchesRelativeToBaseDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "dvb.config"), []byte("CONFIG_DVB=y\n"), 0o644); err != nil {
		t.Fatalf("writing fixture fragment: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "patches"), 0o755); err != nil {
		t.Fatalf("mkdir patches: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "patches", "0001-a.patch"), []byte("patch a\n"), 0o644); err != nil {
		t.Fatalf("writing fixture patch: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "patches", "0002-b.patch"), []byte("patch b\n"), 0o644); err != nil {
		t.Fatalf("writing fixture patch: %v", err)
	}

	cfg, err := kernelconfig.Parse([]byte(`[kernel.radxa-zero-3e]
fragment = "dvb.config"
patches = ["patches/*.patch"]
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	overlay, err := cfg.Overlay("radxa-zero-3e", dir)
	if err != nil {
		t.Fatalf("Overlay: %v", err)
	}
	if string(overlay.ConfigFragment) != "CONFIG_DVB=y\n" {
		t.Errorf("ConfigFragment = %q, want CONFIG_DVB=y", overlay.ConfigFragment)
	}
	if len(overlay.Patches) != 2 {
		t.Fatalf("Patches has %d entries, want 2", len(overlay.Patches))
	}
	if overlay.Patches[0].Name != "0001-a.patch" || overlay.Patches[1].Name != "0002-b.patch" {
		t.Errorf("Patches = %v, want sorted 0001-a.patch, 0002-b.patch", overlay.Patches)
	}
}

func TestOverlayMissingFragmentErrorsNamingThePath(t *testing.T) {
	dir := t.TempDir()
	cfg, err := kernelconfig.Parse([]byte(`[kernel.radxa-zero-3e]
fragment = "does-not-exist.config"
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	_, err = cfg.Overlay("radxa-zero-3e", dir)
	if err == nil {
		t.Fatal("Overlay with a missing fragment succeeded, want an error")
	}
	wantPath := filepath.Join(dir, "does-not-exist.config")
	if !strings.Contains(err.Error(), wantPath) {
		t.Errorf("error = %q, want it to contain the missing path %q", err.Error(), wantPath)
	}
}

func TestOverlayPatchesGlobWithNoMatchesErrors(t *testing.T) {
	dir := t.TempDir()
	cfg, err := kernelconfig.Parse([]byte(`[kernel.radxa-zero-3e]
patches = ["no-such-dir/*.patch"]
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	_, err = cfg.Overlay("radxa-zero-3e", dir)
	if err == nil {
		t.Fatal("Overlay with a non-matching patches glob succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "no-such-dir/*.patch") {
		t.Errorf("error = %q, want it to name the glob pattern", err.Error())
	}
}
