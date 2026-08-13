package cardconfig_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jphastings/gosd/cmd/gosd-init/internal/cardconfig"
	"github.com/jphastings/gosd/internal/configtree"
)

// writeTree lays out a config tree on disk the way a flashed card holds
// one: keys are paths within the tree, values the file contents exactly.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for path, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("creating %s: %v", path, err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("writing %s: %v", path, err)
		}
	}
	return dir
}

func discardLog(string, ...any) {}

func TestReadTrimsPaddingAndTreatsAnEmptyFileAsUnset(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"hostname":        "kitchen-pi\n" + strings.Repeat("\n", 245),
		"wifi/ssid":       strings.Repeat("\n", 256),
		"wifi/passphrase": "hunter2",
	})

	tree := cardconfig.Read(dir, discardLog)

	if got := tree.Get("hostname"); got != "kitchen-pi" {
		t.Errorf("hostname = %q, want the value without its padding", got)
	}
	if got := tree.Get("wifi/ssid"); got != "" {
		t.Errorf("wifi/ssid = %q, want unset: a file of padding alone says nothing", got)
	}
	if got := tree.Get("wifi/passphrase"); got != "hunter2" {
		t.Errorf("wifi/passphrase = %q, want %q", got, "hunter2")
	}
	if got := tree.Get("nothing/here"); got != "" {
		t.Errorf("a setting that isn't on the card = %q, want the same answer as an empty one", got)
	}
}

func TestReadKeepsTheBytesTheCardHoldsAlongsideTheValue(t *testing.T) {
	// The store (and anything else comparing a card against the image it
	// was flashed from) hashes the file as written, padding included —
	// never the value it reads as.
	content := "kitchen-pi\n" + strings.Repeat("\n", 245)
	dir := writeTree(t, map[string]string{"hostname": content})

	tree := cardconfig.Read(dir, discardLog)

	if got := string(tree["hostname"].Content); got != content {
		t.Errorf("hostname bytes = %q, want the file exactly as the card holds it", got)
	}
	if got, want := tree["hostname"].SHA256(), (configtree.Value{Content: []byte(content)}).SHA256(); got != want {
		t.Errorf("digest = %s, want %s (the same digest the build recorded)", got, want)
	}
}

func TestReadIgnoresDocumentationAndOperatingSystemJunk(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"hostname":                 "kitchen-pi",
		"explain.md":               "# Settings",
		"hostname.explain.md":      "# Device name",
		"hostname.new":             "old-default",
		"hostname.unused":          "gone-in-this-image",
		"._hostname":               "a macOS AppleDouble companion",
		".DS_Store":                "\x00\x00",
		"Thumbs.db":                "\x00",
		"wifi/explain.md":          "# WiFi",
		"wifi/ssid":                "HomeNetwork",
		".git/config":              "[core]",
		"env/API_TOKEN":            "s3cret",
		"env/API_TOKEN.explain.md": "# The token",
	})

	tree := cardconfig.Read(dir, discardLog)

	want := []string{"env/API_TOKEN", "hostname", "wifi/ssid"}
	if !slices.Equal(tree.Paths(), want) {
		t.Errorf("settings on the card = %v, want %v", tree.Paths(), want)
	}
}

func TestReadSkipsAFileTooLargeToBeASetting(t *testing.T) {
	// Something dropped into config/ by accident must never be read into
	// the memory of a device whose whole root filesystem is RAM.
	dir := writeTree(t, map[string]string{
		"hostname": "kitchen-pi",
		"holiday":  strings.Repeat("x", cardconfig.MaxValueBytes+1),
	})
	var logged []string
	tree := cardconfig.Read(dir, func(format string, args ...any) { logged = append(logged, format) })

	if _, ok := tree["holiday"]; ok {
		t.Error("an oversized file was read as a setting")
	}
	if tree.Get("hostname") != "kitchen-pi" {
		t.Error("an oversized file stopped the rest of the tree being read")
	}
	if len(logged) != 1 {
		t.Errorf("logged %d lines, want exactly one naming the file that was skipped", len(logged))
	}
}

func TestReadOfAMissingTreeIsAnEmptyTreeAndOneLogLine(t *testing.T) {
	var logged int
	tree := cardconfig.Read(filepath.Join(t.TempDir(), "config"), func(string, ...any) { logged++ })

	if len(tree) != 0 {
		t.Errorf("tree = %v, want no settings at all", tree)
	}
	if logged != 1 {
		t.Errorf("logged %d lines, want exactly one about the missing directory", logged)
	}
}

func TestGroupReturnsOnlyTheSetValuesDirectlyInsideADirectory(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"env/API_TOKEN": "s3cret",
		"env/LOG_LEVEL": "debug",
		"env/UNSET":     "\n\n",
		"wifi/ssid":     "HomeNetwork",
		"hostname":      "kitchen-pi",
	})

	group := cardconfig.Read(dir, discardLog).Group("env")

	want := map[string]string{"API_TOKEN": "s3cret", "LOG_LEVEL": "debug"}
	if len(group) != len(want) {
		t.Fatalf("env group = %v, want %v", group, want)
	}
	for name, value := range want {
		if group[name] != value {
			t.Errorf("env group %s = %q, want %q", name, group[name], value)
		}
	}
}

func TestWritePadsToTheReservationTheFileAlreadyHolds(t *testing.T) {
	// The padding IS the reservation: a device writing a short value over
	// a long one would shrink a region a provisioning tool published.
	dir := writeTree(t, map[string]string{
		"ingress/cloudflared/token": strings.Repeat("\n", 1024),
	})
	tree := cardconfig.Read(dir, discardLog)

	if err := tree.Write(dir, map[string]string{"ingress/cloudflared/token": "tunnel-token"}); err != nil {
		t.Fatalf("Write() = %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "ingress", "cloudflared", "token"))
	if err != nil {
		t.Fatalf("reading the setting back: %v", err)
	}
	if len(content) != 1024 {
		t.Errorf("token file is %d bytes, want its 1024-byte reservation kept", len(content))
	}
	if got := configtree.TrimValue(content); got != "tunnel-token" {
		t.Errorf("token reads as %q, want %q", got, "tunnel-token")
	}
	if got := tree.Get("ingress/cloudflared/token"); got != "tunnel-token" {
		t.Errorf("in-memory token = %q, want the written value", got)
	}
}

func TestWriteCreatesASettingAndItsDirectoryAtTheDefaultReservation(t *testing.T) {
	dir := t.TempDir()
	tree := cardconfig.Tree{}

	if err := tree.Write(dir, map[string]string{"wifi/ssid": "HomeNetwork"}); err != nil {
		t.Fatalf("Write() = %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "wifi", "ssid"))
	if err != nil {
		t.Fatalf("reading the setting back: %v", err)
	}
	if len(content) != configtree.MinValueBytes {
		t.Errorf("ssid file is %d bytes, want the %d-byte reservation every setting gets", len(content), configtree.MinValueBytes)
	}
	if got := configtree.TrimValue(content); got != "HomeNetwork" {
		t.Errorf("ssid reads as %q, want %q", got, "HomeNetwork")
	}
}

func TestWriteKeepsTheValueInMemoryWhenTheCardRefusesIt(t *testing.T) {
	// A card that won't take a setting costs the device the NEXT boot's
	// settings, never this boot's.
	dir := writeTree(t, map[string]string{"wifi": "this is a file, so nothing can live inside it"})
	tree := cardconfig.Tree{}

	err := tree.Write(dir, map[string]string{"wifi/ssid": "HomeNetwork"})

	if err == nil {
		t.Fatal("Write() into a path blocked by a file = nil, want an error")
	}
	if got := tree.Get("wifi/ssid"); got != "HomeNetwork" {
		t.Errorf("in-memory wifi/ssid = %q, want the value to apply to this boot regardless", got)
	}
}

func TestOnCardNamesTheFileSomebodyWouldOpen(t *testing.T) {
	if got, want := cardconfig.OnCard("wifi/ssid"), "config/wifi/ssid"; got != want {
		t.Errorf("OnCard(%q) = %q, want %q", "wifi/ssid", got, want)
	}
}
