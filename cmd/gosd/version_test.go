package main

import (
	"bytes"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/jphastings/gosd/internal/artifacts"
)

// TestVersionReportsTheArtifactsPin covers the reason this command exists:
// which board artifacts a binary downloads is what decides whether an image
// boots, and it is otherwise undiscoverable from an installed gosd (bean
// gosd-7frv).
func TestVersionReportsTheArtifactsPin(t *testing.T) {
	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"version"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("gosd version: %v", err)
	}
	if !strings.Contains(out.String(), artifacts.Version) {
		t.Errorf("version output = %q, want it to name the artifacts pin %q", out.String(), artifacts.Version)
	}
}

// TestVersionFlagMatchesSubcommand pins that `--version` — what people try
// first — reports as much as the subcommand, rather than cobra's bare string.
func TestVersionFlagMatchesSubcommand(t *testing.T) {
	run := func(args ...string) string {
		var out bytes.Buffer
		cmd := newRootCmd()
		cmd.SetOut(&out)
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("gosd %v: %v", args, err)
		}
		return out.String()
	}
	if got, want := run("--version"), run("version"); got != want {
		t.Errorf("gosd --version = %q, want the same as gosd version, %q", got, want)
	}
}

// TestVersionWithoutBuildInfoStillReports guards the degraded case: a binary
// stripped of build information should still say what it can, since a
// version command that fails is less useful than one that admits what it
// doesn't know.
func TestVersionWithoutBuildInfoStillReports(t *testing.T) {
	var out bytes.Buffer
	if err := readVersionInfo(nil, false).write(&out); err != nil {
		t.Fatalf("write: %v", err)
	}

	if !strings.Contains(out.String(), artifacts.Version) {
		t.Errorf("output = %q, want the artifacts pin even with no build info", out.String())
	}
	if !strings.Contains(out.String(), "unknown") {
		t.Errorf("output = %q, want it to say the CLI version is unknown rather than print an empty one", out.String())
	}
}

// TestVersionShowsADirtyCheckout pins that a build from a modified checkout
// says so: "it works on my machine" starts here.
func TestVersionShowsADirtyCheckout(t *testing.T) {
	bi := &debug.BuildInfo{Settings: []debug.BuildSetting{
		{Key: "vcs.revision", Value: "0123456789abcdef0123456789abcdef01234567"},
		{Key: "vcs.modified", Value: "true"},
	}}
	bi.Main.Version = "(devel)"

	var out bytes.Buffer
	if err := readVersionInfo(bi, true).write(&out); err != nil {
		t.Fatalf("write: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "0123456789ab") || !strings.Contains(got, "modified") {
		t.Errorf("output = %q, want the short revision and a modified marker", got)
	}
}
