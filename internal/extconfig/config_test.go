package extconfig_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jphastings/gosd/internal/boards"
	"github.com/jphastings/gosd/internal/container"
	"github.com/jphastings/gosd/internal/extconfig"
)

func TestParseEmptyIsNoop(t *testing.T) {
	cfg, err := extconfig.Parse(nil)
	if err != nil {
		t.Fatalf("Parse(nil): %v", err)
	}
	if len(cfg.Externals) != 0 {
		t.Errorf("Parse(nil).Externals = %v, want empty", cfg.Externals)
	}
}

func TestParseMalformedTOMLErrorsActionably(t *testing.T) {
	_, err := extconfig.Parse([]byte("this is not [ valid toml"))
	if err == nil {
		t.Fatal("Parse(malformed) succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "gosd-external.toml") {
		t.Errorf("error = %q, want it to name gosd-external.toml", err.Error())
	}
}

func TestParseFullSchemaHappyPath(t *testing.T) {
	data := []byte(`
[external.mpv]
script = "build-mpv.sh"
arch = ["arm64", "arm-6"]
image = "docker.io/library/debian:bookworm@sha256:deadbeef"
builder = "docker"

[[external.mpv.source]]
name = "mpv"
repo = "https://github.com/mpv-player/mpv"
ref = "v0.38.0"
license = "GPL-2.0-or-later"
`)
	cfg, err := extconfig.Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	ext, ok := cfg.Externals["mpv"]
	if !ok {
		t.Fatal("Parse did not produce an [external.mpv] entry")
	}
	if ext.Name != "mpv" {
		t.Errorf("Name = %q, want mpv", ext.Name)
	}
	if ext.Script != "build-mpv.sh" {
		t.Errorf("Script = %q, want build-mpv.sh", ext.Script)
	}
	wantArch := []boards.Arch{{GOARCH: "arm64"}, {GOARCH: "arm", GOARM: "6"}}
	if len(ext.Arch) != len(wantArch) || ext.Arch[0] != wantArch[0] || ext.Arch[1] != wantArch[1] {
		t.Errorf("Arch = %v, want %v", ext.Arch, wantArch)
	}
	if ext.Image != "docker.io/library/debian:bookworm@sha256:deadbeef" {
		t.Errorf("Image = %q, want the fixture image", ext.Image)
	}
	if ext.Builder != container.RuntimeDocker {
		t.Errorf("Builder = %q, want %q", ext.Builder, container.RuntimeDocker)
	}

	if len(ext.Sources) != 1 {
		t.Fatalf("Sources has %d entries, want 1", len(ext.Sources))
	}
	want := extconfig.Source{Name: "mpv", Repo: "https://github.com/mpv-player/mpv", Ref: "v0.38.0", License: "GPL-2.0-or-later"}
	if ext.Sources[0] != want {
		t.Errorf("Sources[0] = %+v, want %+v", ext.Sources[0], want)
	}
}

func TestParseMinimalEntryOmitsOptionalFields(t *testing.T) {
	cfg, err := extconfig.Parse([]byte(`
[external.mpv]
script = "build-mpv.sh"
arch = ["arm64"]
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	ext := cfg.Externals["mpv"]
	if ext.Image != "" || ext.Builder != "" || ext.Sources != nil {
		t.Errorf("External = %+v, want empty Image/Builder/Sources when omitted", ext)
	}
}

func TestParseUnknownTopLevelKeyErrorsNamingIt(t *testing.T) {
	_, err := extconfig.Parse([]byte(`oops = "typo"`))
	if err == nil {
		t.Fatal("Parse with an unknown top-level key succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "oops") {
		t.Errorf("error = %q, want it to name the unknown key", err.Error())
	}
}

func TestParseUnknownKeyInsideExternalSectionErrorsNamingIt(t *testing.T) {
	_, err := extconfig.Parse([]byte(`
[external.mpv]
script = "build-mpv.sh"
arch = ["arm64"]
oops = "typo"
`))
	if err == nil {
		t.Fatal("Parse with an unknown key inside an external section succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "oops") {
		t.Errorf("error = %q, want it to name the unknown key", err.Error())
	}
}

func TestParseUnknownKeyInsideSourceEntryErrorsNamingIt(t *testing.T) {
	_, err := extconfig.Parse([]byte(`
[external.mpv]
script = "build-mpv.sh"
arch = ["arm64"]

[[external.mpv.source]]
name = "mpv"
repo = "https://github.com/mpv-player/mpv"
ref = "v0.38.0"
license = "GPL-2.0-or-later"
oops = "typo"
`))
	if err == nil {
		t.Fatal("Parse with an unknown key inside a source entry succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "oops") {
		t.Errorf("error = %q, want it to name the unknown key", err.Error())
	}
}

func TestParseMissingScriptErrorsActionably(t *testing.T) {
	_, err := extconfig.Parse([]byte(`
[external.mpv]
arch = ["arm64"]
`))
	if err == nil {
		t.Fatal("Parse with a missing script succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "script") {
		t.Errorf("error = %q, want it to mention script", err.Error())
	}
}

func TestParseMissingArchErrorsActionably(t *testing.T) {
	_, err := extconfig.Parse([]byte(`
[external.mpv]
script = "build-mpv.sh"
`))
	if err == nil {
		t.Fatal("Parse with a missing arch succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "arch") {
		t.Errorf("error = %q, want it to mention arch", err.Error())
	}
}

func TestParseUnknownArchTokenErrorsListingKnownArches(t *testing.T) {
	_, err := extconfig.Parse([]byte(`
[external.mpv]
script = "build-mpv.sh"
arch = ["riscv64"]
`))
	if err == nil {
		t.Fatal("Parse with an unknown arch token succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "riscv64") {
		t.Errorf("error = %q, want it to name the unknown arch token", err.Error())
	}
	if !strings.Contains(err.Error(), "arm64") {
		t.Errorf("error = %q, want it to list known arch tokens (e.g. arm64)", err.Error())
	}
}

func TestParseInvalidBuilderErrorsActionably(t *testing.T) {
	_, err := extconfig.Parse([]byte(`
[external.mpv]
script = "build-mpv.sh"
arch = ["arm64"]
builder = "crostini"
`))
	if err == nil {
		t.Fatal("Parse with an invalid builder succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "docker") || !strings.Contains(err.Error(), "podman") {
		t.Errorf("error = %q, want it to mention both docker and podman", err.Error())
	}
}

func sourceFixtureMissing(field string) string {
	fields := map[string]string{
		"name":    `name = "mpv"`,
		"repo":    `repo = "https://github.com/mpv-player/mpv"`,
		"ref":     `ref = "v0.38.0"`,
		"license": `license = "GPL-2.0-or-later"`,
	}
	var b strings.Builder
	b.WriteString("[external.mpv]\nscript = \"build-mpv.sh\"\narch = [\"arm64\"]\n\n[[external.mpv.source]]\n")
	for name, line := range fields {
		if name == field {
			continue
		}
		fmt.Fprintln(&b, line)
	}
	return b.String()
}

func TestParseSourceMissingRequiredFieldErrorsActionably(t *testing.T) {
	for _, field := range []string{"name", "repo", "ref", "license"} {
		t.Run(field, func(t *testing.T) {
			_, err := extconfig.Parse([]byte(sourceFixtureMissing(field)))
			if err == nil {
				t.Fatalf("Parse with a missing source %s succeeded, want an error", field)
			}
			if !strings.Contains(err.Error(), field) {
				t.Errorf("error = %q, want it to mention %q", err.Error(), field)
			}
		})
	}
}

func TestScriptPathResolvesRelativeToBaseDir(t *testing.T) {
	cfg, err := extconfig.Parse([]byte(`
[external.mpv]
script = "build/mpv.sh"
arch = ["arm64"]
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got := cfg.Externals["mpv"].ScriptPath("/recipes/mpv")
	want := filepath.Join("/recipes/mpv", "build/mpv.sh")
	if got != want {
		t.Errorf("ScriptPath = %q, want %q", got, want)
	}
}

func TestScriptPathLeavesAbsolutePathUnchanged(t *testing.T) {
	cfg, err := extconfig.Parse([]byte(`
[external.mpv]
script = "/opt/scripts/mpv.sh"
arch = ["arm64"]
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got := cfg.Externals["mpv"].ScriptPath("/recipes/mpv")
	if got != "/opt/scripts/mpv.sh" {
		t.Errorf("ScriptPath = %q, want the absolute path unchanged", got)
	}
}

func TestReadScriptReadsRelativeToBaseDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "build-mpv.sh"), []byte("#!/bin/sh\necho building\n"), 0o755); err != nil {
		t.Fatalf("writing fixture script: %v", err)
	}

	cfg, err := extconfig.Parse([]byte(`
[external.mpv]
script = "build-mpv.sh"
arch = ["arm64"]
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	data, err := cfg.Externals["mpv"].ReadScript(dir)
	if err != nil {
		t.Fatalf("ReadScript: %v", err)
	}
	if string(data) != "#!/bin/sh\necho building\n" {
		t.Errorf("ReadScript content = %q, want the fixture's content", data)
	}
}

func TestReadScriptMissingFileErrorsNamingThePath(t *testing.T) {
	dir := t.TempDir()
	cfg, err := extconfig.Parse([]byte(`
[external.mpv]
script = "does-not-exist.sh"
arch = ["arm64"]
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	_, err = cfg.Externals["mpv"].ReadScript(dir)
	if err == nil {
		t.Fatal("ReadScript with a missing file succeeded, want an error")
	}
	wantPath := filepath.Join(dir, "does-not-exist.sh")
	if !strings.Contains(err.Error(), wantPath) {
		t.Errorf("error = %q, want it to contain the missing path %q", err.Error(), wantPath)
	}
}
