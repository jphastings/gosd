package hostsfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStaticContainsTheConventionalLoopbackLines(t *testing.T) {
	got := Static()
	for _, want := range []string{"127.0.0.1 localhost\n", "::1 localhost ip6-localhost ip6-loopback\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("Static() = %q, want it to contain %q", got, want)
		}
	}
}

func TestRenderIncludesStaticLinesAndTheHostnameLine(t *testing.T) {
	got := Render("my-device")

	if !strings.HasPrefix(got, Static()) {
		t.Errorf("Render(%q) = %q, want it to start with Static() = %q", "my-device", got, Static())
	}
	if !strings.Contains(got, "127.0.1.1 my-device\n") {
		t.Errorf("Render(%q) = %q, want it to contain the 127.0.1.1 hostname line", "my-device", got)
	}
}

func TestRenderChangesOnlyTheHostnameLine(t *testing.T) {
	a := Render("device-a")
	b := Render("device-b")

	if !strings.Contains(a, "127.0.1.1 device-a\n") || !strings.Contains(b, "127.0.1.1 device-b\n") {
		t.Fatalf("Render did not produce the expected hostname lines: %q / %q", a, b)
	}
	if strings.TrimSuffix(a, "127.0.1.1 device-a\n") != strings.TrimSuffix(b, "127.0.1.1 device-b\n") {
		t.Errorf("Render's static portion differs between hostnames: %q vs %q", a, b)
	}
}

func TestWriteProducesRenderedContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hosts")

	if err := Write(path, "my-device"); err != nil {
		t.Fatalf("Write() = %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	if string(got) != Render("my-device") {
		t.Errorf("Write wrote %q, want %q", got, Render("my-device"))
	}
}

func TestWriteOverwritesAPreviouslyDifferentHostnameLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hosts")
	if err := Write(path, "old-name"); err != nil {
		t.Fatalf("seeding %s: %v", path, err)
	}

	if err := Write(path, "new-name"); err != nil {
		t.Fatalf("Write() = %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	if strings.Contains(string(got), "old-name") {
		t.Errorf("hosts file still contains the stale hostname: %q", got)
	}
	if !strings.Contains(string(got), "127.0.1.1 new-name\n") {
		t.Errorf("hosts file missing the new hostname line: %q", got)
	}
	if !strings.Contains(string(got), Static()) {
		t.Errorf("hosts file lost its static localhost lines: %q", got)
	}
}

func TestWritePreservesStaticLinesWhenRewritingOverBakedContent(t *testing.T) {
	// Simulates the real boot sequence: gosd build bakes Static() into the
	// initramfs, then gosd-init calls Write once the hostname settles. The
	// static lines must come out unchanged, not merely re-derived.
	path := filepath.Join(t.TempDir(), "hosts")
	if err := os.WriteFile(path, []byte(Static()), 0o644); err != nil {
		t.Fatalf("seeding baked content at %s: %v", path, err)
	}

	if err := Write(path, "my-device"); err != nil {
		t.Fatalf("Write() = %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	if !strings.HasPrefix(string(got), Static()) {
		t.Errorf("hosts file = %q, want it to still start with the baked static lines %q", got, Static())
	}
}

func TestWriteLeavesNoStrayTempFileOnSuccess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hosts")

	if err := Write(path, "my-device"); err != nil {
		t.Fatalf("Write() = %v", err)
	}

	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("stray temp file left behind: err=%v", err)
	}
}

func TestRenderGivesNoHostnameLineToAValueThatCouldForgeOne(t *testing.T) {
	// The injection bean gosd-39da's payload: a hostname carrying a newline
	// and a second, attacker-chosen mapping. Go's pure resolver reads this
	// file ahead of DNS for every lookup an app makes, so an extra line here
	// is a redirected API endpoint on a device its owner just re-flashed.
	forged := "evil\n1.2.3.4 api.vendor.example"

	got := Render(forged)

	if got != Static() {
		t.Errorf("Render(%q) = %q, want only the static lines", forged, got)
	}
	if strings.Contains(got, "api.vendor.example") {
		t.Errorf("Render(%q) let an attacker-chosen mapping into /etc/hosts: %q", forged, got)
	}
}

func TestRenderRefusesEveryHostnameTheConfigTreeGateWouldRefuse(t *testing.T) {
	for _, name := range []string{
		"",
		"has space",
		"UPPER",
		"trailing\n",
		"nul\x00byte",
		"under_score",
		strings.Repeat("a", 64),
	} {
		if got := Render(name); got != Static() {
			t.Errorf("Render(%q) = %q, want only the static lines", name, got)
		}
	}
}

func TestWriteKeepsTheStaticLinesWhenTheHostnameIsRefused(t *testing.T) {
	// A refused hostname must cost the device its 127.0.1.1 line and
	// nothing else: "localhost" still has to resolve, which is the whole
	// reason this file exists.
	path := filepath.Join(t.TempDir(), "hosts")

	if err := Write(path, "evil\n1.2.3.4 api.vendor.example"); err != nil {
		t.Fatalf("Write() = %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	if string(got) != Static() {
		t.Errorf("hosts file = %q, want exactly the static lines", got)
	}
}
