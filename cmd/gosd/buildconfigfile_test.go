package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/pflag"

	"github.com/jphastings/gosd/internal/buildconfig"
)

// TestFlagKeyParityIsStructural pins the two halves of the locked decision
// (bean gosd-mwct): every gosd build flag is settable from gosd-build.toml,
// and the flag<->key mapping is purely structural — a flag --<section>-<rest>
// whose <section> names a schema table lives at [section].rest, everything
// else at a top-level key of the flag's own name. No hand-maintained map may
// creep in on either side.
func TestFlagKeyParityIsStructural(t *testing.T) {
	keys := make(map[string]bool)
	sections := make(map[string]bool)
	for _, k := range buildconfig.Keys() {
		keys[k] = true
		if section, _, ok := strings.Cut(k, "."); ok {
			sections[section] = true
		}
	}

	flagToKey := func(name string) string {
		if section, rest, ok := strings.Cut(name, "-"); ok && sections[section] {
			return section + "." + rest
		}
		return name
	}

	seen := map[string]bool{"app.main": true} // the positional operand's key; no flag by design
	newBuildCmd().Flags().VisitAll(func(f *pflag.Flag) {
		if f.Name == "build-config" || f.Name == "help" {
			return // CLI-only by design
		}
		want := flagToKey(f.Name)
		if !keys[want] {
			t.Errorf("flag --%s has no gosd-build.toml key (expected %q); add the field to buildconfig.Config", f.Name, want)
		}
		seen[want] = true
	})
	for k := range keys {
		if !seen[k] {
			t.Errorf("gosd-build.toml key %q matches no gosd build flag", k)
		}
	}
}

func TestFileKeyTablesMatchSchemaAndFlags(t *testing.T) {
	schema := make(map[string]bool)
	for _, k := range buildconfig.Keys() {
		schema[k] = true
	}

	buildFlags := newBuildCmd().Flags()
	covered := map[string]bool{"app.main": true} // handled by resolveMainOperand
	for _, fk := range buildFileKeys() {
		if buildFlags.Lookup(fk.flag) == nil {
			t.Errorf("buildFileKeys names flag --%s, which gosd build doesn't register", fk.flag)
		}
		if !schema[fk.key] {
			t.Errorf("buildFileKeys names key %q, which the schema doesn't recognize", fk.key)
		}
		covered[fk.key] = true
	}
	for k := range schema {
		if !covered[k] {
			t.Errorf("schema key %q has no buildFileKeys entry", k)
		}
	}

	runFlags := newRunCmd().Flags()
	for _, fk := range runFileKeys() {
		if runFlags.Lookup(fk.flag) == nil {
			t.Errorf("runFileKeys names flag --%s, which gosd run doesn't register", fk.flag)
		}
		if !schema[fk.key] {
			t.Errorf("runFileKeys names key %q, which the schema doesn't recognize", fk.key)
		}
	}
}

func mustParse(t *testing.T, toml string) buildconfig.Config {
	t.Helper()
	cfg, err := buildconfig.Parse([]byte(toml))
	if err != nil {
		t.Fatalf("Parse errored: %v", err)
	}
	return cfg
}

func TestApplyFileValuesFlagWinsPerKey(t *testing.T) {
	cfg := mustParse(t, `
board = ["pi-zero-2w", "pi-3b"]
label-prefix = "fromfile"
usb-gadget = true

[publish]
catalog = true
`)

	cmd := newBuildCmd()
	if err := cmd.Flags().Parse([]string{"--board", "qemu-virt", "--publish-catalog=false"}); err != nil {
		t.Fatal(err)
	}
	applyFileValues(cmd.Flags(), cfg, "/base", buildFileKeys())

	if len(boardIDs) != 1 || boardIDs[0] != "qemu-virt" {
		t.Errorf("CLI --board should replace the file's array wholesale; got %v", boardIDs)
	}
	if catalogFlag {
		t.Errorf("an explicit --publish-catalog=false should beat the file's catalog = true")
	}
	if labelPrefix != "fromfile" {
		t.Errorf("label-prefix untouched on the CLI should come from the file; got %q", labelPrefix)
	}
	if !usbGadget {
		t.Errorf("usb-gadget untouched on the CLI should come from the file")
	}
	if dataSize != defaultDataSize {
		t.Errorf("a key absent from the file should keep the flag default; got %q", dataSize)
	}
}

func TestApplyFileValuesRebasesFileRelativePaths(t *testing.T) {
	cfg := mustParse(t, `
output = "dist"
artifacts-dir = "/abs/artifacts"
placeholder = ["provision.yaml=32KiB"]
with-external = ["./bin/mpv:/bin/mpv", "helper"]

[boot]
config-dir = "config"

[kernel]
config = "gosd-kernel.toml"
`)

	cmd := newBuildCmd()
	if err := cmd.Flags().Parse(nil); err != nil {
		t.Fatal(err)
	}
	applyFileValues(cmd.Flags(), cfg, "/base", buildFileKeys())

	for got, want := range map[string]string{
		output:        filepath.Join("/base", "dist"),
		artifactsDir:  "/abs/artifacts",
		configDir:     filepath.Join("/base", "config"),
		kernelCfgPath: filepath.Join("/base", "gosd-kernel.toml"),
	} {
		if got != want {
			t.Errorf("rebased path = %q; want %q", got, want)
		}
	}
	if want := filepath.Join("/base", "bin/mpv") + ":/bin/mpv"; withExternal[0] != want {
		t.Errorf("with-external path half = %q; want %q (dest untouched)", withExternal[0], want)
	}
	if want := filepath.Join("/base", "helper"); withExternal[1] != want {
		t.Errorf("bare with-external = %q; want %q", withExternal[1], want)
	}
	if placeholders[0] != "provision.yaml=32KiB" {
		t.Errorf("placeholder is an on-image path and must never be rebased; got %q", placeholders[0])
	}
}

func TestGosdInitSrcEnvBeatsFile(t *testing.T) {
	t.Setenv("GOSD_INIT_SRC", "/env/gosd-init")
	cmd := newBuildCmd() // registered after Setenv: the env default is captured here
	if err := cmd.Flags().Parse(nil); err != nil {
		t.Fatal(err)
	}
	applyFileValues(cmd.Flags(), mustParse(t, `gosd-init-src = "./local"`), "/base", buildFileKeys())
	if gosdInitSrc != "/env/gosd-init" {
		t.Errorf("$GOSD_INIT_SRC must beat the file (a per-machine installation pin); got %q", gosdInitSrc)
	}

	t.Setenv("GOSD_INIT_SRC", "")
	cmd = newBuildCmd()
	if err := cmd.Flags().Parse(nil); err != nil {
		t.Fatal(err)
	}
	applyFileValues(cmd.Flags(), mustParse(t, `gosd-init-src = "./local"`), "/base", buildFileKeys())
	if want := filepath.Join("/base", "local"); gosdInitSrc != want {
		t.Errorf("with no env the file value applies, rebased; got %q want %q", gosdInitSrc, want)
	}
}

func TestResolveMainOperand(t *testing.T) {
	fileMain := mustParse(t, "[app]\nmain = \"./cmd/myapp\"")

	got, err := resolveMainOperand([]string{"./cli/app"}, fileMain, "/base", "gosd build")
	if err != nil || got != "./cli/app" {
		t.Errorf("a positional argument must win: got %q, %v", got, err)
	}

	got, err = resolveMainOperand(nil, fileMain, "/base", "gosd build")
	if err != nil || got != filepath.Join("/base", "cmd/myapp") {
		t.Errorf("file main ./cmd/myapp should rebase onto the file's dir: got %q, %v", got, err)
	}

	for _, passthrough := range []string{"github.com/you/app", "/abs/app"} {
		got, err = resolveMainOperand(nil, mustParse(t, "[app]\nmain = \""+passthrough+"\""), "/base", "gosd build")
		if err != nil || got != passthrough {
			t.Errorf("main %q should pass through unrebased: got %q, %v", passthrough, got, err)
		}
	}

	// A flag-shaped value is not a relative form, so it reaches
	// validatePkgPath raw and keeps its allow-list refusal.
	got, err = resolveMainOperand(nil, mustParse(t, "[app]\nmain = \"-toolexec=evil\""), "/base", "gosd build")
	if err != nil {
		t.Fatalf("resolveMainOperand itself should not reject: %v", err)
	}
	if err := validatePkgPath(got); err == nil {
		t.Errorf("validatePkgPath must refuse a file-supplied %q", got)
	}

	for _, cfg := range []buildconfig.Config{mustParse(t, ""), mustParse(t, "[app]\nmain = \"\"")} {
		_, err = resolveMainOperand(nil, cfg, "/base", "gosd build")
		if err == nil || !strings.Contains(err.Error(), "main = ") || !strings.Contains(err.Error(), "gosd build .") {
			t.Errorf("no positional and no usable main must error naming both remedies; got %v", err)
		}
	}
}

func TestLoadBuildConfig(t *testing.T) {
	t.Run("explicit path must exist", func(t *testing.T) {
		_, _, err := loadBuildConfig(filepath.Join(t.TempDir(), "nope.toml"))
		if err == nil || !strings.Contains(err.Error(), "--build-config") {
			t.Errorf("want an actionable --build-config error; got %v", err)
		}
	})

	t.Run("missing default is a zero config", func(t *testing.T) {
		t.Chdir(t.TempDir())
		cfg, baseDir, err := loadBuildConfig("")
		if err != nil || baseDir != "" || cfg.IsSet("board") {
			t.Errorf("missing gosd-build.toml must be a silent zero config; got %+v, %q, %v", cfg, baseDir, err)
		}
	})

	t.Run("default is discovered in the cwd with an absolute base dir", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		if err := os.WriteFile(defaultBuildConfigFile, []byte("[boot]\nsize = \"128MiB\""), 0o644); err != nil {
			t.Fatal(err)
		}
		cfg, baseDir, err := loadBuildConfig("")
		if err != nil || cfg.Boot.Size != "128MiB" {
			t.Fatalf("discovery failed: %+v, %v", cfg, err)
		}
		if !filepath.IsAbs(baseDir) {
			t.Errorf("baseDir must be absolute so rebased paths survive any cwd; got %q", baseDir)
		}
	})

	t.Run("a differently-named file is named in parse errors", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "other.toml")
		if err := os.WriteFile(path, []byte("nonsense-key = 1"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, _, err := loadBuildConfig(path)
		if err == nil || !strings.Contains(err.Error(), "other.toml") {
			t.Errorf("want the actual file named; got %v", err)
		}
	})
}
