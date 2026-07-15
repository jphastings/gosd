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
	"github.com/jphastings/gosd/internal/extbuild"
	"github.com/jphastings/gosd/internal/extconfig"
)

func TestNewBuildExternalCmdFlagDefaults(t *testing.T) {
	cmd := newBuildExternalCmd()

	if got := cmd.Use; got != "build-external" {
		t.Errorf("Use = %q, want build-external", got)
	}
	for _, name := range []string{"config", "name", "arch", "output", "builder"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("flag %q is not registered", name)
		}
	}
	if got := cmd.Flags().Lookup("config").DefValue; got != defaultExternalConfigFile {
		t.Errorf("--config default = %q, want %q", got, defaultExternalConfigFile)
	}
	if got := cmd.Flags().Lookup("output").DefValue; got != defaultExternalOutputDir {
		t.Errorf("--output default = %q, want %q", got, defaultExternalOutputDir)
	}
	if got := cmd.Flags().Lookup("builder").DefValue; got != "" {
		t.Errorf("--builder default = %q, want empty (auto-detect)", got)
	}
}

func TestLoadExternalConfigMissingFileErrorsNamingPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.toml")

	_, _, err := loadExternalConfig(path)
	if err == nil {
		t.Fatal("loadExternalConfig with a missing file succeeded, want an error")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error = %q, want it to name %q", err.Error(), path)
	}
}

func TestLoadExternalConfigMalformedTOMLErrorsActionably(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.toml")
	if err := os.WriteFile(path, []byte("this is not [ valid toml"), 0o644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	_, _, err := loadExternalConfig(path)
	if err == nil {
		t.Fatal("loadExternalConfig with malformed TOML succeeded, want an error")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error = %q, want it to name %q", err.Error(), path)
	}
}

func TestLoadExternalConfigNoRecipesErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.toml")
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	_, _, err := loadExternalConfig(path)
	if err == nil {
		t.Fatal("loadExternalConfig with no [external.*] recipes succeeded, want an error")
	}
}

func TestLoadExternalConfigHappyPathReturnsConfigDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gosd-external.toml")
	if err := os.WriteFile(path, []byte(`[external.mpv]
script = "build.sh"
arch = ["arm64"]
`), 0o644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	cfg, cfgDir, err := loadExternalConfig(path)
	if err != nil {
		t.Fatalf("loadExternalConfig: %v", err)
	}
	if _, ok := cfg.Externals["mpv"]; !ok {
		t.Errorf("cfg.Externals = %v, want an mpv entry", cfg.Externals)
	}
	if cfgDir != dir {
		t.Errorf("cfgDir = %q, want %q", cfgDir, dir)
	}
}

func externalFixture(name string, arches ...boards.Arch) extconfig.External {
	return extconfig.External{Name: name, Script: "build.sh", Arch: arches}
}

// externalCfgDir returns a temp directory containing a build.sh, standing
// in for the directory gosd-external.toml itself lives in: buildExternals
// resolves each recipe's Script against it via extconfig.External.ReadScript.
func externalCfgDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "build.sh"), []byte("#!/bin/sh\necho building\n"), 0o755); err != nil {
		t.Fatalf("writing fixture build.sh: %v", err)
	}
	return dir
}

func TestResolveExternalsAllByDefaultInSortedOrder(t *testing.T) {
	cfg := extconfig.Config{Externals: map[string]extconfig.External{
		"zeta": externalFixture("zeta", boards.KnownArches["arm64"]),
		"alfa": externalFixture("alfa", boards.KnownArches["arm64"]),
	}}

	selected, err := resolveExternals(cfg, nil)
	if err != nil {
		t.Fatalf("resolveExternals(nil): %v", err)
	}
	if len(selected) != 2 || selected[0].Name != "alfa" || selected[1].Name != "zeta" {
		t.Errorf("selected = %v, want [alfa zeta] in sorted order", selected)
	}
}

func TestResolveExternalsNameFilter(t *testing.T) {
	cfg := extconfig.Config{Externals: map[string]extconfig.External{
		"mpv":      externalFixture("mpv", boards.KnownArches["arm64"]),
		"gpiotool": externalFixture("gpiotool", boards.KnownArches["arm64"]),
	}}

	selected, err := resolveExternals(cfg, []string{"mpv", "mpv"})
	if err != nil {
		t.Fatalf("resolveExternals: %v", err)
	}
	if len(selected) != 1 || selected[0].Name != "mpv" {
		t.Errorf("selected = %v, want exactly [mpv] (repeats deduped)", selected)
	}
}

func TestResolveExternalsUnknownNameErrorsListingKnown(t *testing.T) {
	cfg := extconfig.Config{Externals: map[string]extconfig.External{
		"mpv": externalFixture("mpv", boards.KnownArches["arm64"]),
	}}

	_, err := resolveExternals(cfg, []string{"nope"})
	if err == nil {
		t.Fatal("resolveExternals with an unknown --name succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "nope") || !strings.Contains(err.Error(), "mpv") {
		t.Errorf("error = %q, want it to name both the unknown value and the known externals", err.Error())
	}
}

func TestResolveArchTokensEmptyMeansNoFilter(t *testing.T) {
	arches, err := resolveArchTokens(nil)
	if err != nil {
		t.Fatalf("resolveArchTokens(nil): %v", err)
	}
	if arches != nil {
		t.Errorf("resolveArchTokens(nil) = %v, want nil", arches)
	}
}

func TestResolveArchTokensValidatesAndDedupes(t *testing.T) {
	arches, err := resolveArchTokens([]string{"arm64", "arm64", "arm-6"})
	if err != nil {
		t.Fatalf("resolveArchTokens: %v", err)
	}
	if len(arches) != 2 {
		t.Errorf("arches = %v, want 2 deduped entries", arches)
	}
}

func TestResolveArchTokensUnknownArchErrors(t *testing.T) {
	_, err := resolveArchTokens([]string{"mips"})
	if err == nil {
		t.Fatal("resolveArchTokens([mips]) succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "mips") {
		t.Errorf("error = %q, want it to name mips", err.Error())
	}
}

func TestArchesForExternalNoFilterReturnsEveryDeclaredArch(t *testing.T) {
	ext := externalFixture("mpv", boards.KnownArches["arm64"], boards.KnownArches["arm-6"])

	arches, err := archesForExternal(ext, nil)
	if err != nil {
		t.Fatalf("archesForExternal: %v", err)
	}
	if len(arches) != 2 {
		t.Errorf("arches = %v, want both declared arches", arches)
	}
}

func TestArchesForExternalIntersectsFilter(t *testing.T) {
	ext := externalFixture("mpv", boards.KnownArches["arm64"], boards.KnownArches["arm-6"])

	arches, err := archesForExternal(ext, []boards.Arch{boards.KnownArches["arm64"]})
	if err != nil {
		t.Fatalf("archesForExternal: %v", err)
	}
	if len(arches) != 1 || arches[0].Key() != "arm64" {
		t.Errorf("arches = %v, want exactly [arm64]", arches)
	}
}

func TestArchesForExternalEmptyIntersectionErrorsNamingExternal(t *testing.T) {
	ext := externalFixture("gpiotool", boards.KnownArches["arm64"])

	_, err := archesForExternal(ext, []boards.Arch{boards.KnownArches["arm-6"]})
	if err == nil {
		t.Fatal("archesForExternal with no overlap succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "gpiotool") {
		t.Errorf("error = %q, want it to name gpiotool", err.Error())
	}
}

func TestEffectiveExternalBuilderPrefFlagWinsOverRecipe(t *testing.T) {
	if got := effectiveExternalBuilderPref(container.RuntimeDocker, container.RuntimePodman); got != container.RuntimeDocker {
		t.Errorf("effectiveExternalBuilderPref(docker, podman) = %q, want docker (flag wins)", got)
	}
	if got := effectiveExternalBuilderPref("", container.RuntimePodman); got != container.RuntimePodman {
		t.Errorf("effectiveExternalBuilderPref(\"\", podman) = %q, want podman (falls back to recipe)", got)
	}
	if got := effectiveExternalBuilderPref("", ""); got != "" {
		t.Errorf("effectiveExternalBuilderPref(\"\", \"\") = %q, want empty (auto-detect)", got)
	}
}

// fakeExternalBuild returns an externalBuildFunc that writes a fixture
// binary and source.json into a fresh temp cache dir per call - mirroring
// the shape internal/extbuild.Build's cache entries take - so
// writeExternalOutput's copy step has real files to read, without ever
// running a container. skip reports whether a given call should report a
// cache hit.
func fakeExternalBuild(t *testing.T, skip func(spec extbuild.Spec) bool) (*[]extbuild.Spec, externalBuildFunc) {
	t.Helper()
	var calls []extbuild.Spec
	fn := func(_ context.Context, spec extbuild.Spec, _ extbuild.Options) (extbuild.Result, error) {
		calls = append(calls, spec)

		dir := t.TempDir()
		content := "binary-for-" + spec.Name + "-" + spec.Arch.Key()
		if err := os.WriteFile(filepath.Join(dir, spec.Name), []byte(content), 0o755); err != nil {
			t.Fatalf("writing fixture binary: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "source.json"), []byte(`[{"Name":"`+spec.Name+`"}]`), 0o644); err != nil {
			t.Fatalf("writing fixture source.json: %v", err)
		}

		skipped := skip != nil && skip(spec)
		return extbuild.Result{CacheDir: dir, OutputPath: filepath.Join(dir, spec.Name), Skipped: skipped}, nil
	}
	return &calls, fn
}

func fakeDetect(rt containerRuntime, err error) detectRuntimeFunc {
	return func(context.Context, string) (containerRuntime, error) { return rt, err }
}

func TestBuildExternalsWritesOutputPerArchAndName(t *testing.T) {
	ext := externalFixture("mpv", boards.KnownArches["arm64"], boards.KnownArches["arm-6"])
	_, build := fakeExternalBuild(t, nil)
	outputDir := filepath.Join(t.TempDir(), "gosd-externals")

	outcomes, err := buildExternals(context.Background(), []extconfig.External{ext}, externalCfgDir(t), nil, "",
		fakeDetect(fakeContainerRuntime{}, nil), build, outputDir, &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("buildExternals: %v", err)
	}
	if len(outcomes) != 2 {
		t.Fatalf("outcomes = %v, want one per declared arch", outcomes)
	}

	for _, archKey := range []string{"arm64", "arm-6"} {
		binPath := filepath.Join(outputDir, archKey, "mpv")
		if _, err := os.Stat(binPath); err != nil {
			t.Errorf("binary not written to %s: %v", binPath, err)
		}
		jsonPath := filepath.Join(outputDir, archKey, "mpv.source.json")
		if _, err := os.Stat(jsonPath); err != nil {
			t.Errorf("source.json not written to %s: %v", jsonPath, err)
		}
	}
}

func TestBuildExternalsSourceJSONDoesNotCollideAcrossNames(t *testing.T) {
	mpv := externalFixture("mpv", boards.KnownArches["arm64"])
	gpiotool := externalFixture("gpiotool", boards.KnownArches["arm64"])
	_, build := fakeExternalBuild(t, nil)
	outputDir := filepath.Join(t.TempDir(), "gosd-externals")

	_, err := buildExternals(context.Background(), []extconfig.External{mpv, gpiotool}, externalCfgDir(t), nil, "",
		fakeDetect(fakeContainerRuntime{}, nil), build, outputDir, &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("buildExternals: %v", err)
	}

	mpvJSON, err := os.ReadFile(filepath.Join(outputDir, "arm64", "mpv.source.json"))
	if err != nil {
		t.Fatalf("reading mpv.source.json: %v", err)
	}
	gpioJSON, err := os.ReadFile(filepath.Join(outputDir, "arm64", "gpiotool.source.json"))
	if err != nil {
		t.Fatalf("reading gpiotool.source.json: %v", err)
	}
	if string(mpvJSON) == string(gpioJSON) {
		t.Errorf("mpv.source.json and gpiotool.source.json have identical content %q, want each to carry its own provenance", mpvJSON)
	}
	if !strings.Contains(string(mpvJSON), "mpv") || !strings.Contains(string(gpioJSON), "gpiotool") {
		t.Errorf("source.json files = %q / %q, want each to name its own external", mpvJSON, gpioJSON)
	}
}

func TestBuildExternalsReportsCacheHit(t *testing.T) {
	ext := externalFixture("mpv", boards.KnownArches["arm64"])
	_, build := fakeExternalBuild(t, func(extbuild.Spec) bool { return true })

	outcomes, err := buildExternals(context.Background(), []extconfig.External{ext}, externalCfgDir(t), nil, "",
		fakeDetect(fakeContainerRuntime{}, nil), build, t.TempDir(), &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("buildExternals: %v", err)
	}
	if len(outcomes) != 1 || !outcomes[0].skipped {
		t.Errorf("outcomes = %+v, want a single cache-hit build", outcomes)
	}
}

func TestBuildExternalsArchFilterAppliesPerRecipe(t *testing.T) {
	mpv := externalFixture("mpv", boards.KnownArches["arm64"], boards.KnownArches["arm-6"])
	calls, build := fakeExternalBuild(t, nil)

	_, err := buildExternals(context.Background(), []extconfig.External{mpv}, externalCfgDir(t), []boards.Arch{boards.KnownArches["arm-6"]}, "",
		fakeDetect(fakeContainerRuntime{}, nil), build, t.TempDir(), &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("buildExternals: %v", err)
	}
	if len(*calls) != 1 || (*calls)[0].Arch.Key() != "arm-6" {
		t.Errorf("calls = %v, want a single arm-6 build", *calls)
	}
}

func TestBuildExternalsAbortsWhenArchFilterMatchesNoRecipe(t *testing.T) {
	gpiotool := externalFixture("gpiotool", boards.KnownArches["arm64"])
	calls, build := fakeExternalBuild(t, nil)

	_, err := buildExternals(context.Background(), []extconfig.External{gpiotool}, externalCfgDir(t), []boards.Arch{boards.KnownArches["arm-6"]}, "",
		fakeDetect(fakeContainerRuntime{}, nil), build, t.TempDir(), &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("buildExternals succeeded despite an --arch filter matching no declared arch, want an error")
	}
	if len(*calls) != 0 {
		t.Errorf("build was called %d times, want 0 (filtered out before any build)", len(*calls))
	}
}

func TestBuildExternalsPerRecipeBuilderPrecedence(t *testing.T) {
	dockerExt := extconfig.External{Name: "mpv", Script: "build.sh", Arch: []boards.Arch{boards.KnownArches["arm64"]}, Builder: container.RuntimeDocker}
	podmanExt := extconfig.External{Name: "gpiotool", Script: "build.sh", Arch: []boards.Arch{boards.KnownArches["arm64"]}, Builder: container.RuntimePodman}
	_, build := fakeExternalBuild(t, nil)

	var seenPrefs []string
	detect := func(_ context.Context, preferred string) (containerRuntime, error) {
		seenPrefs = append(seenPrefs, preferred)
		return fakeContainerRuntime{}, nil
	}

	_, err := buildExternals(context.Background(), []extconfig.External{dockerExt, podmanExt}, externalCfgDir(t), nil, "",
		detect, build, t.TempDir(), &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("buildExternals: %v", err)
	}
	if len(seenPrefs) != 2 || seenPrefs[0] != container.RuntimeDocker || seenPrefs[1] != container.RuntimePodman {
		t.Errorf("seenPrefs = %v, want [docker podman] (each recipe's own builder, no --builder flag override)", seenPrefs)
	}
}

func TestBuildExternalsBuilderFlagOverridesRecipe(t *testing.T) {
	ext := extconfig.External{Name: "mpv", Script: "build.sh", Arch: []boards.Arch{boards.KnownArches["arm64"]}, Builder: container.RuntimePodman}
	_, build := fakeExternalBuild(t, nil)

	var seenPrefs []string
	detect := func(_ context.Context, preferred string) (containerRuntime, error) {
		seenPrefs = append(seenPrefs, preferred)
		return fakeContainerRuntime{}, nil
	}

	_, err := buildExternals(context.Background(), []extconfig.External{ext}, externalCfgDir(t), nil, container.RuntimeDocker,
		detect, build, t.TempDir(), &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("buildExternals: %v", err)
	}
	if len(seenPrefs) != 1 || seenPrefs[0] != container.RuntimeDocker {
		t.Errorf("seenPrefs = %v, want [docker] (--builder flag overrides the recipe's own podman)", seenPrefs)
	}
}

func TestBuildExternalsDockerMissingErrorPropagatesActionably(t *testing.T) {
	ext := externalFixture("mpv", boards.KnownArches["arm64"])
	_, build := fakeExternalBuild(t, nil)

	notInstalled := &container.NotInstalledError{Command: "gosd build-external"}
	_, err := buildExternals(context.Background(), []extconfig.External{ext}, externalCfgDir(t), nil, "",
		fakeDetect(nil, notInstalled), build, t.TempDir(), &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("buildExternals succeeded despite no container runtime, want an error")
	}
	if !strings.Contains(err.Error(), "gosd build-external") {
		t.Errorf("error = %q, want it to name gosd build-external (not gosd build-kernel)", err.Error())
	}
}

func TestPrintExternalBuildSummaryReportsStatusAndHint(t *testing.T) {
	cmd := &cobra.Command{}
	var buf bytes.Buffer
	cmd.SetErr(&buf)

	printExternalBuildSummary(cmd, []externalBuildOutcome{
		{name: "mpv", arch: "arm64", skipped: false, path: "/tmp/gosd-externals/arm64/mpv"},
		{name: "mpv", arch: "arm-6", skipped: true, path: "/tmp/gosd-externals/arm-6/mpv"},
	}, "/tmp/gosd-externals")

	out := buf.String()
	if !strings.Contains(out, "cache hit") {
		t.Errorf("summary = %q, want it to report the arm-6 build as a cache hit", out)
	}
	if !strings.Contains(out, "gosd build --with-external /tmp/gosd-externals/arm64/mpv") {
		t.Errorf("summary = %q, want it to hint the --with-external follow-up", out)
	}
}

func TestPrintExternalBuildSummaryHandlesNoOutcomes(t *testing.T) {
	cmd := &cobra.Command{}
	var buf bytes.Buffer
	cmd.SetErr(&buf)

	printExternalBuildSummary(cmd, nil, "/tmp/gosd-externals")

	if !strings.Contains(buf.String(), "nothing was built") {
		t.Errorf("summary = %q, want it to say nothing was built", buf.String())
	}
}
