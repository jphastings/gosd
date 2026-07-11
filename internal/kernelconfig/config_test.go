package kernelconfig_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jphastings/gosd/internal/artifacts"
	"github.com/jphastings/gosd/internal/container"
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

// validSHA256 is a syntactically valid (if fictitious) 64-character hex
// digest, used by every firmware fixture below that doesn't specifically
// test sha256 validation itself.
var validSHA256 = strings.Repeat("ab", 32)

func TestParseFullSchemaHappyPath(t *testing.T) {
	data := []byte(fmt.Sprintf(`
[kernel]
based-on = %q
builder = "docker"

[kernel.radxa-zero-3e]
fragment = "fragments/dvb.config"
patches = ["patches/*.patch"]

[[firmware]]
url = "https://example.com/blob.fw"
sha256 = %q
dest = "vendor/blob.fw"
`, artifacts.Version, validSHA256))

	cfg, err := kernelconfig.Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if cfg.BasedOn != artifacts.Version {
		t.Errorf("BasedOn = %q, want %q", cfg.BasedOn, artifacts.Version)
	}
	if cfg.Builder != container.RuntimeDocker {
		t.Errorf("Builder = %q, want %q", cfg.Builder, container.RuntimeDocker)
	}

	board, ok := cfg.Kernel["radxa-zero-3e"]
	if !ok {
		t.Fatal("Kernel has no radxa-zero-3e entry")
	}
	if board.Fragment != "fragments/dvb.config" || len(board.Patches) != 1 || board.Patches[0] != "patches/*.patch" {
		t.Errorf("board = %+v, want the fragment/patches from the fixture", board)
	}

	if len(cfg.Firmware) != 1 {
		t.Fatalf("Firmware has %d entries, want 1", len(cfg.Firmware))
	}
	fw := cfg.Firmware[0]
	if fw.URL != "https://example.com/blob.fw" || fw.SHA256 != validSHA256 || fw.Dest != "vendor/blob.fw" {
		t.Errorf("Firmware[0] = %+v, want the fixture's url/sha256/dest", fw)
	}
}

func TestParseUnknownTopLevelKeyErrorsNamingIt(t *testing.T) {
	_, err := kernelconfig.Parse([]byte(`oops = "typo"`))
	if err == nil {
		t.Fatal("Parse with an unknown top-level key succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "oops") {
		t.Errorf("error = %q, want it to name the unknown key", err.Error())
	}
}

func TestParseUnknownKeyInsideBoardSectionErrorsNamingIt(t *testing.T) {
	_, err := kernelconfig.Parse([]byte(`
[kernel.radxa-zero-3e]
fragment = "dvb.config"
oops = "typo"
`))
	if err == nil {
		t.Fatal("Parse with an unknown key inside a board section succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "oops") {
		t.Errorf("error = %q, want it to name the unknown key", err.Error())
	}
}

func TestParseUnknownKeyInsideFirmwareEntryErrorsNamingIt(t *testing.T) {
	data := []byte(fmt.Sprintf(`
[[firmware]]
url = "https://example.com/blob.fw"
sha256 = %q
dest = "vendor/blob.fw"
oops = "typo"
`, validSHA256))

	_, err := kernelconfig.Parse(data)
	if err == nil {
		t.Fatal("Parse with an unknown key inside a firmware entry succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "oops") {
		t.Errorf("error = %q, want it to name the unknown key", err.Error())
	}
}

func TestParseUnknownBoardIDErrorsListingKnownIDs(t *testing.T) {
	_, err := kernelconfig.Parse([]byte(`
[kernel.not-a-board]
fragment = "dvb.config"
`))
	if err == nil {
		t.Fatal("Parse with an unknown board ID succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "not-a-board") {
		t.Errorf("error = %q, want it to name the unknown board ID", err.Error())
	}
	if !strings.Contains(err.Error(), "pi-zero-2w") {
		t.Errorf("error = %q, want it to list known board IDs (e.g. pi-zero-2w)", err.Error())
	}
}

func TestParseReservedModuleErrorsActionably(t *testing.T) {
	_, err := kernelconfig.Parse([]byte(`
[[module]]
name = "usb-dvb"
`))
	if err == nil {
		t.Fatal("Parse with [[module]] present succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "reserved") {
		t.Errorf("error = %q, want it to say [[module]] is reserved", err.Error())
	}
	if !strings.Contains(err.Error(), "gosd-2k9p") {
		t.Errorf("error = %q, want it to reference the loadable-modules decision bean", err.Error())
	}
}

func TestParseBasedOnMismatchErrorsActionably(t *testing.T) {
	_, err := kernelconfig.Parse([]byte(`
[kernel]
based-on = "v0.0.1-does-not-exist"
`))
	if err == nil {
		t.Fatal("Parse with a mismatched based-on succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "based-on") {
		t.Errorf("error = %q, want it to mention based-on", err.Error())
	}
	if !strings.Contains(err.Error(), artifacts.Version) {
		t.Errorf("error = %q, want it to name the pinned artifacts version %q", err.Error(), artifacts.Version)
	}
}

func TestParseInvalidBuilderErrorsActionably(t *testing.T) {
	_, err := kernelconfig.Parse([]byte(`
[kernel]
builder = "crostini"
`))
	if err == nil {
		t.Fatal("Parse with an invalid builder succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "docker") || !strings.Contains(err.Error(), "podman") {
		t.Errorf("error = %q, want it to mention both docker and podman", err.Error())
	}
}

func TestParseFirmwareMissingURLErrorsActionably(t *testing.T) {
	data := []byte(fmt.Sprintf(`
[[firmware]]
sha256 = %q
dest = "vendor/blob.fw"
`, validSHA256))

	_, err := kernelconfig.Parse(data)
	if err == nil {
		t.Fatal("Parse with a missing firmware url succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "url") {
		t.Errorf("error = %q, want it to mention url", err.Error())
	}
}

func TestParseFirmwareMissingDestErrorsActionably(t *testing.T) {
	data := []byte(fmt.Sprintf(`
[[firmware]]
url = "https://example.com/blob.fw"
sha256 = %q
`, validSHA256))

	_, err := kernelconfig.Parse(data)
	if err == nil {
		t.Fatal("Parse with a missing firmware dest succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "dest") {
		t.Errorf("error = %q, want it to mention dest", err.Error())
	}
}

func TestParseFirmwareBadSHA256ErrorsActionably(t *testing.T) {
	_, err := kernelconfig.Parse([]byte(`
[[firmware]]
url = "https://example.com/blob.fw"
sha256 = "not-a-digest"
dest = "vendor/blob.fw"
`))
	if err == nil {
		t.Fatal("Parse with a malformed sha256 succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "sha256") {
		t.Errorf("error = %q, want it to mention sha256", err.Error())
	}
}

func TestParseFirmwareAbsoluteDestErrorsActionably(t *testing.T) {
	data := []byte(fmt.Sprintf(`
[[firmware]]
url = "https://example.com/blob.fw"
sha256 = %q
dest = "/etc/passwd"
`, validSHA256))

	_, err := kernelconfig.Parse(data)
	if err == nil {
		t.Fatal("Parse with an absolute firmware dest succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "dest") {
		t.Errorf("error = %q, want it to mention dest", err.Error())
	}
}

func TestParseFirmwareDotDotDestErrorsActionably(t *testing.T) {
	data := []byte(fmt.Sprintf(`
[[firmware]]
url = "https://example.com/blob.fw"
sha256 = %q
dest = "../../etc/passwd"
`, validSHA256))

	_, err := kernelconfig.Parse(data)
	if err == nil {
		t.Fatal("Parse with a \"..\"-escaping firmware dest succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "dest") {
		t.Errorf("error = %q, want it to mention dest", err.Error())
	}
}
