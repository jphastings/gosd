package build

import (
	"os"
	"strings"
	"testing"
)

// TestMinGoVersionMatchesGoMod keeps MinGoVersion honest against go.mod's
// own `go` directive (dependency-derived - see MinGoVersion's docstring):
// if a dependency bump raises the floor, this test fails until MinGoVersion
// is updated to match.
func TestMinGoVersionMatchesGoMod(t *testing.T) {
	data, err := os.ReadFile("../../go.mod")
	if err != nil {
		t.Fatalf("reading go.mod: %v", err)
	}

	for _, line := range strings.Split(string(data), "\n") {
		after, ok := strings.CutPrefix(line, "go ")
		if !ok {
			continue
		}
		if want := "go" + strings.TrimSpace(after); want != MinGoVersion {
			t.Errorf("MinGoVersion = %q, want %q (go.mod's `go` directive)", MinGoVersion, want)
		}
		return
	}
	t.Fatal("go.mod has no `go` directive")
}

func TestCheckToolchainReportsMissingGo(t *testing.T) {
	realPath := os.Getenv("PATH")

	t.Setenv("PATH", "")
	err := CheckToolchain()
	if err == nil {
		t.Fatal("CheckToolchain() with an empty PATH succeeded, want an actionable error")
	}
	for _, want := range []string{MinGoVersion, "https://go.dev/dl", "nix run github:jphastings/gosd"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to mention %q", err, want)
		}
	}

	t.Setenv("PATH", realPath)
	if err := CheckToolchain(); err != nil {
		t.Errorf("CheckToolchain() with the real PATH = %v, want nil", err)
	}
}

func TestExplainBuildFailureAddsUpgradeAdviceForToolchainFloor(t *testing.T) {
	cases := []struct {
		name       string
		stderr     string
		wantAdvice bool
	}{
		{
			name:       "go.mod floor",
			stderr:     "go: go.mod requires go >= 1.26.5 (running go 1.26.4; GOTOOLCHAIN=local)\n",
			wantAdvice: true,
		},
		{
			name:       "dependency floor",
			stderr:     "go: module tailscale.com@v1.102.2 requires go >= 1.26.5 (running go 1.26.4)\n",
			wantAdvice: true,
		},
		{
			name:       "ordinary compiler error",
			stderr:     "./main.go:7:2: undefined: foo\n",
			wantAdvice: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := explainBuildFailure("building ./cmd/myapp for linux/arm64 failed", "go build ./cmd/myapp", c.stderr)

			baseline := "building ./cmd/myapp for linux/arm64 failed; try running `go build ./cmd/myapp` directly to reproduce:\n" + c.stderr
			if !strings.HasPrefix(err.Error(), baseline) {
				t.Errorf("error = %q, want it to start with the baseline shape %q", err, baseline)
			}

			hasAdvice := strings.Contains(err.Error(), "https://go.dev/dl") && strings.Contains(err.Error(), "GOTOOLCHAIN=local")
			if hasAdvice != c.wantAdvice {
				t.Errorf("error = %q, want upgrade advice = %v", err, c.wantAdvice)
			}

			if !c.wantAdvice && err.Error() != baseline {
				t.Errorf("error = %q, want exactly the baseline shape with no extra text", err)
			}
		})
	}
}

// TestExplainBuildFailureQuotesNoVersionOfItsOwn pins the reason the
// remediation names no version: the floor that tripped may be the user's
// app's, not gosd's, so quoting MinGoVersion would contradict the Go stderr
// printed directly above it.
func TestExplainBuildFailureQuotesNoVersionOfItsOwn(t *testing.T) {
	stderr := "go: go.mod requires go >= 1.27.0 (running go 1.26.5; GOTOOLCHAIN=local)\n"

	err := explainBuildFailure("building ./cmd/myapp for linux/arm64 failed", "go build ./cmd/myapp", stderr)

	advice := strings.TrimPrefix(err.Error(), "building ./cmd/myapp for linux/arm64 failed; try running `go build ./cmd/myapp` directly to reproduce:\n"+stderr)
	if strings.Contains(advice, MinGoVersion) {
		t.Errorf("remediation = %q, want it to name no version (stderr already names 1.27.0)", advice)
	}
}
