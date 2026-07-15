package extbuild_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jphastings/gosd/internal/boards"
	"github.com/jphastings/gosd/internal/container"
	"github.com/jphastings/gosd/internal/extbuild"
)

func testSpec() extbuild.Spec {
	return extbuild.Spec{
		Name:   "mpv",
		Script: []byte("#!/bin/sh\necho building mpv\n"),
		Arch:   boards.Arch{GOARCH: "arm64"},
		Sources: []extbuild.Source{
			{Name: "mpv", Repo: "https://github.com/mpv-player/mpv", Ref: "v0.38.0", License: "GPL-2.0-or-later"},
		},
	}
}

// fakeRunner is a container-runner test double. By default it simulates a
// successful build by writing a fixture static ELF binary matching spec's
// arch into the mount at /out/<spec.Name>; runFn can be set to override
// that behavior (e.g. to simulate a RunFailedError, or a script that
// produces the wrong output).
type fakeRunner struct {
	calls []container.RunSpec
	runFn func(spec container.RunSpec) error
}

func newSucceedingRunner(spec extbuild.Spec) *fakeRunner {
	return &fakeRunner{
		runFn: func(runSpec container.RunSpec) error {
			return writeFakeOutput(runSpec, spec.Name, spec.Arch)
		},
	}
}

func (f *fakeRunner) Run(_ context.Context, spec container.RunSpec) error {
	f.calls = append(f.calls, spec)
	if f.runFn == nil {
		return nil
	}
	return f.runFn(spec)
}

func mountHostPath(spec container.RunSpec, containerPath string) string {
	for _, m := range spec.Mounts {
		if m.ContainerPath == containerPath {
			return m.HostPath
		}
	}
	return ""
}

func writeFakeOutput(runSpec container.RunSpec, name string, arch boards.Arch) error {
	outDir := mountHostPath(runSpec, "/out")
	if outDir == "" {
		return errors.New("no /out mount found in RunSpec")
	}
	return writeStaticELFFixture(filepath.Join(outDir, name), arch)
}

func buildWithFakeRunner(t *testing.T, spec extbuild.Spec, opts extbuild.Options) (extbuild.Result, error) {
	t.Helper()
	if opts.Runtime == nil {
		opts.Runtime = newSucceedingRunner(spec)
	}
	if opts.CacheDir == "" {
		opts.CacheDir = t.TempDir()
	}
	return extbuild.Build(context.Background(), spec, opts)
}

func TestBuild_RunsContainerAndCollectsOutput(t *testing.T) {
	spec := testSpec()
	rt := newSucceedingRunner(spec)
	outDir := t.TempDir()

	result, err := buildWithFakeRunner(t, spec, extbuild.Options{Runtime: rt, OutputDir: outDir})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if result.Skipped {
		t.Fatal("Skipped = true on a fresh cache, want false")
	}
	if len(rt.calls) != 1 {
		t.Fatalf("container was run %d times, want 1", len(rt.calls))
	}

	for _, name := range []string{spec.Name, "source.json"} {
		if _, err := os.Stat(filepath.Join(outDir, name)); err != nil {
			t.Errorf("output dir missing %s: %v", name, err)
		}
	}
	if result.OutputPath != filepath.Join(outDir, spec.Name) {
		t.Errorf("OutputPath = %q, want %q", result.OutputPath, filepath.Join(outDir, spec.Name))
	}
}

func TestBuild_ContainerContractEnvAndMounts(t *testing.T) {
	spec := testSpec()
	rt := newSucceedingRunner(spec)

	if _, err := buildWithFakeRunner(t, spec, extbuild.Options{Runtime: rt}); err != nil {
		t.Fatalf("Build: %v", err)
	}

	call := rt.calls[0]
	if call.Env["GOSD_ARCH"] != "arm64" {
		t.Errorf("GOSD_ARCH = %q, want arm64", call.Env["GOSD_ARCH"])
	}
	if call.Env["GOSD_CROSS_COMPILE"] != "aarch64-linux-gnu-" {
		t.Errorf("GOSD_CROSS_COMPILE = %q, want aarch64-linux-gnu-", call.Env["GOSD_CROSS_COMPILE"])
	}
	if call.Env["GOSD_OUTPUT"] != "/out/mpv" {
		t.Errorf("GOSD_OUTPUT = %q, want /out/mpv", call.Env["GOSD_OUTPUT"])
	}

	var work, out *container.Mount
	for i := range call.Mounts {
		m := &call.Mounts[i]
		switch m.ContainerPath {
		case "/work":
			work = m
		case "/out":
			out = m
		}
	}
	if work == nil || !work.ReadOnly {
		t.Errorf("/work mount = %+v, want present and read-only", work)
	}
	if out == nil || out.ReadOnly {
		t.Errorf("/out mount = %+v, want present and read-write", out)
	}
}

func TestBuild_ArmV6UsesArmToolchain(t *testing.T) {
	spec := testSpec()
	spec.Arch = boards.Arch{GOARCH: "arm", GOARM: "6"}
	rt := newSucceedingRunner(spec)

	if _, err := buildWithFakeRunner(t, spec, extbuild.Options{Runtime: rt}); err != nil {
		t.Fatalf("Build: %v", err)
	}

	call := rt.calls[0]
	if call.Env["GOSD_ARCH"] != "arm-6" {
		t.Errorf("GOSD_ARCH = %q, want arm-6", call.Env["GOSD_ARCH"])
	}
	if call.Env["GOSD_CROSS_COMPILE"] != "arm-linux-gnueabihf-" {
		t.Errorf("GOSD_CROSS_COMPILE = %q, want arm-linux-gnueabihf-", call.Env["GOSD_CROSS_COMPILE"])
	}
}

// TestBuild_WorkDirCarriesScriptAndWrapper reads the generated files from
// inside runFn, while /work is still on disk - Build removes its work dir
// once the container "run" returns (mirrors kernelbuild's captureBuild
// test helper).
func TestBuild_WorkDirCarriesScriptAndWrapper(t *testing.T) {
	spec := testSpec()
	var script, wrapper []byte
	rt := &fakeRunner{runFn: func(runSpec container.RunSpec) error {
		workDir := mountHostPath(runSpec, "/work")
		var err error
		if script, err = os.ReadFile(filepath.Join(workDir, "script.sh")); err != nil {
			return err
		}
		if wrapper, err = os.ReadFile(filepath.Join(workDir, "wrapper.sh")); err != nil {
			return err
		}
		return writeFakeOutput(runSpec, spec.Name, spec.Arch)
	}}

	if _, err := buildWithFakeRunner(t, spec, extbuild.Options{Runtime: rt}); err != nil {
		t.Fatalf("Build: %v", err)
	}

	if string(script) != string(spec.Script) {
		t.Errorf("script.sh = %q, want the spec's Script verbatim", script)
	}
	for _, want := range []string{"bash /work/script.sh", "GOSD_OUTPUT", "mpv"} {
		if !strings.Contains(string(wrapper), want) {
			t.Errorf("wrapper.sh missing %q:\n%s", want, wrapper)
		}
	}
}

// The /work bind mount must come from under the cache dir, never
// os.TempDir(): macOS's default temp dir (/var/folders/…) isn't shared with
// Docker Desktop's VM, so a mount from there is silently empty in the
// container (gosd-0p21, same failure mode kernelbuild guards against).
func TestBuild_WorkDirLivesUnderCacheDir(t *testing.T) {
	spec := testSpec()
	rt := newSucceedingRunner(spec)
	cacheDir := t.TempDir()

	if _, err := extbuild.Build(context.Background(), spec, extbuild.Options{Runtime: rt, CacheDir: cacheDir}); err != nil {
		t.Fatalf("Build: %v", err)
	}

	workHost := mountHostPath(rt.calls[0], "/work")
	if workHost == "" {
		t.Fatal("no /work mount in the container RunSpec")
	}
	if rel, err := filepath.Rel(cacheDir, workHost); err != nil || strings.HasPrefix(rel, "..") {
		t.Errorf("/work mounted from %s, want a directory under the cache dir %s", workHost, cacheDir)
	}
}

func TestBuild_CacheHitSkipsContainer(t *testing.T) {
	spec := testSpec()
	rt := newSucceedingRunner(spec)
	cacheDir := t.TempDir()

	if _, err := extbuild.Build(context.Background(), spec, extbuild.Options{Runtime: rt, CacheDir: cacheDir}); err != nil {
		t.Fatalf("first Build: %v", err)
	}
	if len(rt.calls) != 1 {
		t.Fatalf("after first build, container ran %d times, want 1", len(rt.calls))
	}

	outDir := t.TempDir()
	result, err := extbuild.Build(context.Background(), spec, extbuild.Options{Runtime: rt, CacheDir: cacheDir, OutputDir: outDir})
	if err != nil {
		t.Fatalf("second Build: %v", err)
	}
	if !result.Skipped {
		t.Error("Skipped = false on repeat build with identical inputs, want true")
	}
	if len(rt.calls) != 1 {
		t.Fatalf("container ran %d times after cache-hit build, want still 1 (no re-run)", len(rt.calls))
	}
	if _, err := os.Stat(filepath.Join(outDir, spec.Name)); err != nil {
		t.Errorf("cache-hit build did not collect output: %v", err)
	}
}

func TestBuild_CacheMissesOnChangedScript(t *testing.T) {
	base := testSpec()
	cacheDir := t.TempDir()

	baseKey := buildAndGetKey(t, base, cacheDir)

	changed := base
	changed.Script = []byte("#!/bin/sh\necho a different script\n")
	changedKey := buildAndGetKey(t, changed, cacheDir)

	if baseKey == changedKey {
		t.Error("cache key unchanged after the script's bytes changed")
	}
}

func TestBuild_CacheMissesOnImageChange(t *testing.T) {
	spec := testSpec()
	cacheDir := t.TempDir()

	rt1 := newSucceedingRunner(spec)
	r1, err := extbuild.Build(context.Background(), spec, extbuild.Options{Runtime: rt1, CacheDir: cacheDir, Image: "image-a"})
	if err != nil {
		t.Fatalf("build with image-a: %v", err)
	}

	rt2 := newSucceedingRunner(spec)
	r2, err := extbuild.Build(context.Background(), spec, extbuild.Options{Runtime: rt2, CacheDir: cacheDir, Image: "image-b"})
	if err != nil {
		t.Fatalf("build with image-b: %v", err)
	}

	if r1.CacheKey == r2.CacheKey {
		t.Error("cache key unchanged after changing the container image")
	}
	if len(rt2.calls) != 1 {
		t.Errorf("container ran %d times for image-b, want 1 (should not have hit image-a's cache entry)", len(rt2.calls))
	}
}

func TestBuild_CacheMissesOnArchChange(t *testing.T) {
	base := testSpec()
	cacheDir := t.TempDir()
	baseKey := buildAndGetKey(t, base, cacheDir)

	armv6 := base
	armv6.Arch = boards.Arch{GOARCH: "arm", GOARM: "6"}
	armv6Key := buildAndGetKey(t, armv6, cacheDir)

	if baseKey == armv6Key {
		t.Error("cache key unchanged after changing the target arch")
	}
}

func buildAndGetKey(t *testing.T, spec extbuild.Spec, cacheDir string) string {
	t.Helper()
	rt := newSucceedingRunner(spec)
	result, err := extbuild.Build(context.Background(), spec, extbuild.Options{Runtime: rt, CacheDir: cacheDir})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return result.CacheKey
}

// If the staging dir is deleted on the host mid-build (cache cleaner, macOS
// storage-pressure eviction - gosd-l4y9), the error must say that plainly
// instead of surfacing the container's confusing ENOENT.
func TestBuild_StagingDirVanishingMidBuildIsExplained(t *testing.T) {
	for name, runErr := range map[string]error{"run reports failure": errors.New("exit 1"), "run reports success": nil} {
		t.Run(name, func(t *testing.T) {
			rt := &fakeRunner{runFn: func(runSpec container.RunSpec) error {
				if err := os.RemoveAll(mountHostPath(runSpec, "/out")); err != nil {
					t.Fatal(err)
				}
				return runErr
			}}
			_, err := buildWithFakeRunner(t, testSpec(), extbuild.Options{Runtime: rt})
			if err == nil || !strings.Contains(err.Error(), "disappeared while the build was running") {
				t.Errorf("error = %v, want the vanished-staging explanation", err)
			}
		})
	}
}

func TestBuild_InterruptedBuildLeavesNoCacheEntry(t *testing.T) {
	cacheDir := t.TempDir()
	rt := &fakeRunner{runFn: func(container.RunSpec) error {
		return errors.New("simulated container failure")
	}}

	_, err := extbuild.Build(context.Background(), testSpec(), extbuild.Options{Runtime: rt, CacheDir: cacheDir})
	if err == nil {
		t.Fatal("Build succeeded despite the runner failing")
	}

	entries, readErr := os.ReadDir(cacheDir)
	if readErr != nil {
		t.Fatalf("reading cache dir: %v", readErr)
	}
	if len(entries) != 0 {
		t.Errorf("cache dir has %d entries after an interrupted build, want 0: %v", len(entries), entries)
	}
}

func TestBuild_MissingOutputIsAnError(t *testing.T) {
	rt := &fakeRunner{runFn: func(runSpec container.RunSpec) error {
		// Simulate a script that "succeeds" but forgets to write GOSD_OUTPUT.
		return nil
	}}

	_, err := extbuild.Build(context.Background(), testSpec(), extbuild.Options{Runtime: rt, CacheDir: t.TempDir()})
	if err == nil {
		t.Fatal("Build succeeded despite a missing expected output file")
	}
}

func TestBuild_ArchMismatchOutputIsRejected(t *testing.T) {
	spec := testSpec() // arm64
	rt := &fakeRunner{runFn: func(runSpec container.RunSpec) error {
		outDir := mountHostPath(runSpec, "/out")
		// Write an armv6 (32-bit) binary where an arm64 one was expected.
		return writeStaticELFFixture(filepath.Join(outDir, spec.Name), boards.Arch{GOARCH: "arm", GOARM: "6"})
	}}

	_, err := extbuild.Build(context.Background(), spec, extbuild.Options{Runtime: rt, CacheDir: t.TempDir()})
	if err == nil {
		t.Fatal("Build succeeded despite an arch-mismatched output, want an error")
	}
	if !strings.Contains(err.Error(), "verification") {
		t.Errorf("error = %q, want it to mention verification", err.Error())
	}
}

func TestBuild_DynamicallyLinkedOutputIsRejected(t *testing.T) {
	spec := testSpec()
	rt := &fakeRunner{runFn: func(runSpec container.RunSpec) error {
		outDir := mountHostPath(runSpec, "/out")
		return writeDynamicELFFixture(filepath.Join(outDir, spec.Name))
	}}

	_, err := extbuild.Build(context.Background(), spec, extbuild.Options{Runtime: rt, CacheDir: t.TempDir()})
	if err == nil {
		t.Fatal("Build succeeded despite a dynamically linked output, want an error")
	}
	if !strings.Contains(err.Error(), "static") {
		t.Errorf("error = %q, want it to explain the binary must be static", err.Error())
	}
}

func TestBuild_RequiresRuntime(t *testing.T) {
	_, err := extbuild.Build(context.Background(), testSpec(), extbuild.Options{})
	if err == nil {
		t.Fatal("Build succeeded with a nil Runtime")
	}
}

func TestBuild_RequiresName(t *testing.T) {
	spec := testSpec()
	spec.Name = ""
	_, err := buildWithFakeRunner(t, spec, extbuild.Options{})
	if err == nil {
		t.Fatal("Build succeeded with an empty Spec.Name")
	}
}

func TestBuild_RejectsNameWithSlash(t *testing.T) {
	spec := testSpec()
	spec.Name = "sub/mpv"
	_, err := buildWithFakeRunner(t, spec, extbuild.Options{})
	if err == nil {
		t.Fatal("Build succeeded with a Spec.Name containing a slash")
	}
}

func TestBuild_RequiresScript(t *testing.T) {
	spec := testSpec()
	spec.Script = nil
	_, err := buildWithFakeRunner(t, spec, extbuild.Options{})
	if err == nil {
		t.Fatal("Build succeeded with an empty Spec.Script")
	}
}
