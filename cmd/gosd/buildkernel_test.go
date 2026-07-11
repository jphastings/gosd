package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/jphastings/gosd/internal/boards"
	"github.com/jphastings/gosd/internal/container"
	"github.com/jphastings/gosd/internal/kernelbuild"
	"github.com/jphastings/gosd/internal/kernelconfig"
	"github.com/jphastings/gosd/internal/kernelspec"
)

// fakeContainerRuntime is a containerRuntime test double: it never shells
// out, so these tests need no Docker/Podman installation.
type fakeContainerRuntime struct{}

func (fakeContainerRuntime) Run(context.Context, container.RunSpec) error { return nil }

func TestNewBuildKernelCmdFlagDefaults(t *testing.T) {
	cmd := newBuildKernelCmd()

	if got := cmd.Use; got != "build-kernel" {
		t.Errorf("Use = %q, want build-kernel", got)
	}
	for _, name := range []string{"board", "output", "config", "builder", "staging"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("flag %q is not registered", name)
		}
	}
	if got := cmd.Flags().Lookup("output").DefValue; got != defaultKernelOutputDir {
		t.Errorf("--output default = %q, want %q", got, defaultKernelOutputDir)
	}
	if got := cmd.Flags().Lookup("builder").DefValue; got != "" {
		t.Errorf("--builder default = %q, want empty (auto-detect)", got)
	}
}

func TestValidateBuilderPref(t *testing.T) {
	for _, ok := range []string{"", "docker", "podman"} {
		if err := validateBuilderPref(ok); err != nil {
			t.Errorf("validateBuilderPref(%q): %v", ok, err)
		}
	}

	err := validateBuilderPref("crostini")
	if err == nil {
		t.Fatal("validateBuilderPref(\"crostini\") succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "docker") || !strings.Contains(err.Error(), "podman") {
		t.Errorf("error = %q, want it to mention both docker and podman", err.Error())
	}
}

func TestLoadKernelConfigDefaultsToNoOverlayWhenFileAbsent(t *testing.T) {
	t.Chdir(t.TempDir())

	cfg, dir, err := loadKernelConfig("")
	if err != nil {
		t.Fatalf("loadKernelConfig(\"\"): %v", err)
	}
	if len(cfg.Kernel) != 0 {
		t.Errorf("cfg.Kernel = %v, want empty when no gosd-kernel.toml exists", cfg.Kernel)
	}
	if dir != "" {
		t.Errorf("cfgDir = %q, want empty", dir)
	}
}

func TestLoadKernelConfigUsesDefaultFileWhenPresent(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile(defaultKernelConfigFile, []byte(`[kernel.radxa-zero-3e]
fragment = "dvb.config"
`), 0o644); err != nil {
		t.Fatalf("writing fixture %s: %v", defaultKernelConfigFile, err)
	}

	cfg, _, err := loadKernelConfig("")
	if err != nil {
		t.Fatalf("loadKernelConfig(\"\"): %v", err)
	}
	if _, ok := cfg.Kernel["radxa-zero-3e"]; !ok {
		t.Errorf("cfg.Kernel = %v, want a radxa-zero-3e entry picked up from the default file", cfg.Kernel)
	}
}

func TestLoadKernelConfigExplicitPathOverridesDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "custom-kernel.toml")
	if err := os.WriteFile(path, []byte(`[kernel.pi-zero-2w]
fragment = "frag.config"
`), 0o644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	cfg, cfgDir, err := loadKernelConfig(path)
	if err != nil {
		t.Fatalf("loadKernelConfig(%q): %v", path, err)
	}
	if _, ok := cfg.Kernel["pi-zero-2w"]; !ok {
		t.Errorf("cfg.Kernel = %v, want a pi-zero-2w entry", cfg.Kernel)
	}
	if cfgDir != dir {
		t.Errorf("cfgDir = %q, want %q", cfgDir, dir)
	}
}

func TestLoadKernelConfigMissingExplicitPathErrorsNamingPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.toml")

	_, _, err := loadKernelConfig(path)
	if err == nil {
		t.Fatal("loadKernelConfig with a missing explicit path succeeded, want an error")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error = %q, want it to name %q", err.Error(), path)
	}
}

func TestLoadKernelConfigMalformedTOMLErrorsActionably(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.toml")
	if err := os.WriteFile(path, []byte("this is not [ valid toml"), 0o644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	_, _, err := loadKernelConfig(path)
	if err == nil {
		t.Fatal("loadKernelConfig with malformed TOML succeeded, want an error")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error = %q, want it to name %q", err.Error(), path)
	}
}

func TestBuildKernelsForBoardsThreadsOutputStagingAndOverlay(t *testing.T) {
	selected := []boards.Board{mustFindBoard(t, "pi-zero-2w")}
	cfg, err := kernelconfig.Parse([]byte(`[kernel.pi-zero-2w]
fragment = "extra.config"
`))
	if err != nil {
		t.Fatalf("kernelconfig.Parse: %v", err)
	}
	cfgDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(cfgDir, "extra.config"), []byte("CONFIG_EXTRA=y\n"), 0o644); err != nil {
		t.Fatalf("writing fixture fragment: %v", err)
	}

	outputDir := filepath.Join(t.TempDir(), "artifacts")
	stagingDir := filepath.Join(t.TempDir(), "staging")

	var gotOutputs kernelbuild.Outputs
	var gotOverlay kernelbuild.Overlay
	fakeBuild := func(_ context.Context, spec kernelspec.KernelSpec, overlay kernelbuild.Overlay, opts kernelbuild.Options) (kernelbuild.Result, error) {
		if spec.BoardID != "pi-zero-2w" {
			t.Errorf("spec.BoardID = %q, want pi-zero-2w", spec.BoardID)
		}
		gotOutputs = opts.Outputs
		gotOverlay = overlay
		return kernelbuild.Result{Skipped: false}, nil
	}

	outcomes, err := buildKernelsForBoards(context.Background(), selected, cfg, cfgDir, fakeContainerRuntime{}, fakeBuild,
		outputDir, stagingDir, &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("buildKernelsForBoards: %v", err)
	}

	if gotOutputs.FlatDir != outputDir {
		t.Errorf("Outputs.FlatDir = %q, want %q", gotOutputs.FlatDir, outputDir)
	}
	if gotOutputs.StagingDir != stagingDir {
		t.Errorf("Outputs.StagingDir = %q, want %q", gotOutputs.StagingDir, stagingDir)
	}
	if string(gotOverlay.ConfigFragment) != "CONFIG_EXTRA=y\n" {
		t.Errorf("overlay.ConfigFragment = %q, want the fragment file's contents", gotOverlay.ConfigFragment)
	}
	if len(outcomes) != 1 || outcomes[0].board != "pi-zero-2w" || outcomes[0].skipped {
		t.Errorf("outcomes = %+v, want a single fresh pi-zero-2w build", outcomes)
	}

	if _, err := os.Stat(outputDir); err != nil {
		t.Errorf("output directory %s was not created: %v", outputDir, err)
	}
}

func TestBuildKernelsForBoardsReportsCacheHit(t *testing.T) {
	selected := []boards.Board{mustFindBoard(t, "qemu-virt")}
	fakeBuild := func(context.Context, kernelspec.KernelSpec, kernelbuild.Overlay, kernelbuild.Options) (kernelbuild.Result, error) {
		return kernelbuild.Result{Skipped: true}, nil
	}

	outcomes, err := buildKernelsForBoards(context.Background(), selected, kernelconfig.Config{}, "", fakeContainerRuntime{}, fakeBuild,
		t.TempDir(), "", &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("buildKernelsForBoards: %v", err)
	}
	if len(outcomes) != 1 || !outcomes[0].skipped {
		t.Errorf("outcomes = %+v, want a single cache-hit qemu-virt build", outcomes)
	}
}

func TestBuildKernelsForBoardsAbortsOnFailureNamingTheBoard(t *testing.T) {
	selected := []boards.Board{mustFindBoard(t, "pi-zero-2w"), mustFindBoard(t, "radxa-zero-3e")}

	var calls []string
	fakeBuild := func(_ context.Context, spec kernelspec.KernelSpec, _ kernelbuild.Overlay, _ kernelbuild.Options) (kernelbuild.Result, error) {
		calls = append(calls, spec.BoardID)
		if spec.BoardID == "pi-zero-2w" {
			return kernelbuild.Result{}, &container.RunFailedError{Runtime: "docker", Image: "gosd-kernel-build", ExitCode: 1}
		}
		return kernelbuild.Result{}, nil
	}

	_, err := buildKernelsForBoards(context.Background(), selected, kernelconfig.Config{}, "", fakeContainerRuntime{}, fakeBuild,
		t.TempDir(), "", &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("buildKernelsForBoards succeeded despite a simulated build failure, want an error")
	}
	if !strings.Contains(err.Error(), "pi-zero-2w") {
		t.Errorf("error = %q, want it to name the failing board pi-zero-2w", err.Error())
	}
	if len(calls) != 1 {
		t.Errorf("build was called for boards %v, want only [pi-zero-2w] (abort on first failure)", calls)
	}
}

func TestBuildKernelsForBoardsCoversDefaultPublicBoardSet(t *testing.T) {
	selected, err := resolveBoards(nil)
	if err != nil {
		t.Fatalf("resolveBoards(nil): %v", err)
	}

	var built []string
	fakeBuild := func(_ context.Context, spec kernelspec.KernelSpec, _ kernelbuild.Overlay, _ kernelbuild.Options) (kernelbuild.Result, error) {
		built = append(built, spec.BoardID)
		return kernelbuild.Result{}, nil
	}

	if _, err := buildKernelsForBoards(context.Background(), selected, kernelconfig.Config{}, "", fakeContainerRuntime{}, fakeBuild,
		t.TempDir(), "", &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("buildKernelsForBoards: %v", err)
	}

	for _, id := range boards.IDs() {
		found := false
		for _, b := range built {
			if b == id {
				found = true
			}
		}
		if !found {
			t.Errorf("default board set did not build %s", id)
		}
	}
	for _, b := range built {
		if b == "qemu-virt" {
			t.Error("default board set built qemu-virt, want it excluded (internal-only)")
		}
	}
}

func TestBuildKernelsForBoardsExplicitQemuVirtBuilds(t *testing.T) {
	selected, err := resolveBoards([]string{"qemu-virt"})
	if err != nil {
		t.Fatalf("resolveBoards([qemu-virt]): %v", err)
	}

	var built []string
	fakeBuild := func(_ context.Context, spec kernelspec.KernelSpec, _ kernelbuild.Overlay, _ kernelbuild.Options) (kernelbuild.Result, error) {
		built = append(built, spec.BoardID)
		return kernelbuild.Result{}, nil
	}

	if _, err := buildKernelsForBoards(context.Background(), selected, kernelconfig.Config{}, "", fakeContainerRuntime{}, fakeBuild,
		t.TempDir(), "", &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("buildKernelsForBoards: %v", err)
	}
	if len(built) != 1 || built[0] != "qemu-virt" {
		t.Errorf("built = %v, want exactly [qemu-virt]", built)
	}
}

func TestPrintKernelBuildSummaryMentionsArtifactsDirFollowUp(t *testing.T) {
	cmd := &cobra.Command{}
	var buf bytes.Buffer
	cmd.SetErr(&buf)

	printKernelBuildSummary(cmd, []kernelBuildOutcome{
		{board: "pi-zero-2w", skipped: false},
		{board: "radxa-zero-3e", skipped: true},
	}, "/tmp/gosd-artifacts")

	out := buf.String()
	if !strings.Contains(out, "gosd build --artifacts-dir /tmp/gosd-artifacts") {
		t.Errorf("summary = %q, want it to mention the gosd build --artifacts-dir follow-up", out)
	}
	if !strings.Contains(out, "pi-zero-2w") || !strings.Contains(out, "radxa-zero-3e") {
		t.Errorf("summary = %q, want it to mention both boards", out)
	}
	if !strings.Contains(out, "cache hit") {
		t.Errorf("summary = %q, want it to report radxa-zero-3e as a cache hit", out)
	}
}
