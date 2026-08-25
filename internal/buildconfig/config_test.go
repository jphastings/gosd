package buildconfig

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseEmptyIsZeroConfig(t *testing.T) {
	for _, data := range [][]byte{nil, {}} {
		cfg, err := Parse(data)
		if err != nil {
			t.Fatalf("Parse(%v) errored: %v", data, err)
		}
		if cfg.IsSet("board") || cfg.IsSet("app.main") {
			t.Fatalf("zero Config reports keys as set")
		}
	}
}

func TestParseFullFile(t *testing.T) {
	got, err := Parse([]byte(`
board = ["pi-zero-2w", "radxa-zero-3e"]
output = "dist"
label-prefix = "myapp"
ingress = ["tailscale-funnel"]
placeholder = ["provision.yaml=32KiB"]
with-external = ["./third_party/mpv:/bin/mpv"]
usb-gadget = true
console-baud = 115200
artifacts-dir = "gosd-artifacts"
gosd-init-src = "../gosd-init"
ldflags = "-X main.version=1.4.2"
tags = "sometag"
trimpath = true
gcflags = "-m"
asmflags = "-D FOO=1"

[app]
main = "./cmd/myapp"
version = "1.4.2"
support-url = "https://example.com/support"

[boot]
size = "256MiB"
config-dir = "config"

[data]
size = "512MiB"
filesystem = "fat32"
flush = true

[kernel]
param = ["snd_bcm2835.enable_hdmi=1"]
config = "gosd-kernel.toml"

[publish]
catalog = true
base-url = "https://example.com/downloads"
`))
	if err != nil {
		t.Fatalf("Parse errored: %v", err)
	}

	want := Config{
		Board:        []string{"pi-zero-2w", "radxa-zero-3e"},
		Output:       "dist",
		LabelPrefix:  "myapp",
		Ingress:      []string{"tailscale-funnel"},
		Placeholder:  []string{"provision.yaml=32KiB"},
		WithExternal: []string{"./third_party/mpv:/bin/mpv"},
		UsbGadget:    true,
		ConsoleBaud:  115200,
		ArtifactsDir: "gosd-artifacts",
		GosdInitSrc:  "../gosd-init",
		LDFlags:      "-X main.version=1.4.2",
		Tags:         "sometag",
		TrimPath:     true,
		GCFlags:      "-m",
		ASMFlags:     "-D FOO=1",
		App:          App{Main: "./cmd/myapp", Version: "1.4.2", SupportURL: "https://example.com/support"},
		Boot:         Boot{Size: "256MiB", ConfigDir: "config"},
		Data:         Data{Size: "512MiB", Filesystem: "fat32", Flush: true},
		Kernel:       Kernel{Param: []string{"snd_bcm2835.enable_hdmi=1"}, Config: "gosd-kernel.toml"},
		Publish:      Publish{Catalog: true, BaseURL: "https://example.com/downloads"},
	}
	want.defined = got.defined
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Parse mismatch:\ngot  %+v\nwant %+v", got, want)
	}

	for _, key := range Keys() {
		if !got.IsSet(key) {
			t.Errorf("full file: IsSet(%q) = false", key)
		}
	}
}

func TestParseUnknownKeysAreNamed(t *testing.T) {
	cases := []struct {
		toml    string
		wantKey string
	}{
		{`boards = ["pi-zero-2w"]`, `"boards"`},                   // top-level typo
		{"[boot]\nsizes = \"256MiB\"", `"boot.sizes"`},            // typo inside a section
		{"[build]\nboard = [\"pi-zero-2w\"]", `"build"`},          // invented section
		{"[data.overrides]\nsize = \"1GiB\"", `"data.overrides"`}, // nested table where a value belongs
	}
	for _, c := range cases {
		_, err := Parse([]byte(c.toml))
		if err == nil || !strings.Contains(err.Error(), c.wantKey) {
			t.Errorf("Parse(%q) error = %v; want it to name %s", c.toml, err, c.wantKey)
		}
	}
}

func TestParseTypeMismatchNamesTheKey(t *testing.T) {
	_, err := Parse([]byte(`board = "pi-zero-2w"`))
	if err == nil || !strings.Contains(err.Error(), "board") {
		t.Fatalf("Parse error = %v; want it to name the key board", err)
	}
	if !strings.Contains(err.Error(), "gosd-build.toml") {
		t.Fatalf("Parse error = %v; want it to name the file", err)
	}
}

func TestIsSetDistinguishesWrittenZeroFromAbsent(t *testing.T) {
	cfg, err := Parse([]byte("label-prefix = \"\"\n\n[publish]\ncatalog = false"))
	if err != nil {
		t.Fatalf("Parse errored: %v", err)
	}
	for _, set := range []string{"label-prefix", "publish.catalog"} {
		if !cfg.IsSet(set) {
			t.Errorf("IsSet(%q) = false for a key written as its zero value", set)
		}
	}
	for _, unset := range []string{"board", "publish.base-url", "app.main"} {
		if cfg.IsSet(unset) {
			t.Errorf("IsSet(%q) = true for an absent key", unset)
		}
	}
}

func TestKeysAreStructuralAndUnique(t *testing.T) {
	keys := Keys()
	seen := make(map[string]bool)
	for _, k := range keys {
		if seen[k] {
			t.Errorf("Keys() repeats %q", k)
		}
		seen[k] = true
	}
	// Spot-check the two mapping shapes; the exhaustive flag<->key check
	// lives in cmd/gosd's structural parity test.
	for _, want := range []string{"board", "label-prefix", "app.main", "boot.config-dir", "publish.base-url"} {
		if !seen[want] {
			t.Errorf("Keys() is missing %q", want)
		}
	}
}

func TestResolvePath(t *testing.T) {
	abs := filepath.Join(string(filepath.Separator), "somewhere", "app")
	if got := ResolvePath("/base", abs); got != abs {
		t.Errorf("ResolvePath passed an absolute path = %q; want it untouched", got)
	}
	if got, want := ResolvePath("/base", "dist"), filepath.Join("/base", "dist"); got != want {
		t.Errorf("ResolvePath(/base, dist) = %q; want %q", got, want)
	}
}
