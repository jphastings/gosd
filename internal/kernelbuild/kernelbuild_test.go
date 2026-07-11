package kernelbuild_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jphastings/gosd/internal/container"
	"github.com/jphastings/gosd/internal/kernelbuild"
	"github.com/jphastings/gosd/internal/kernelspec"
)

// testSpec is a small, made-up KernelSpec exercising every field the
// generator and orchestrator branch on: a DTB, DTS patches, a CommitRef
// source, RequiredY/ForbiddenY, ModulesDisabled and reproducibility pins.
func testSpec() kernelspec.KernelSpec {
	return kernelspec.KernelSpec{
		BoardID: "test-board",
		Source: kernelspec.Source{
			Repo:       "https://example.com/linux.git",
			Ref:        "deadbeefcafef00d",
			RefKind:    kernelspec.CommitRef,
			CommitDate: "2026-01-01T00:00:00Z",
		},
		Defconfig: "test_defconfig",
		Toolchain: kernelspec.Toolchain{KernelArch: "arm64", CrossCompile: "aarch64-linux-gnu-"},

		ConfigFragment: []byte("CONFIG_FOO=y\n"),
		DTSPatches: []kernelspec.Patch{
			{Name: "0001-a.patch", Content: []byte("gosd patch a\n")},
			{Name: "0002-b.patch", Content: []byte("gosd patch b\n")},
		},

		DTB: &kernelspec.DTB{
			MakeTarget: "test.dtb",
			SourcePath: "arch/arm64/boot/dts/test.dtb",
			Filename:   "test-board.dtb",
		},

		KernelMakeTarget: "Image",
		KernelSourcePath: "arch/arm64/boot/Image",
		KernelFilename:   "Image",

		RequiredY:       []string{"CONFIG_FOO=y", "CONFIG_BAR"},
		ForbiddenY:      []string{"CONFIG_BAZ"},
		ModulesDisabled: true,

		Reproducibility: kernelspec.Reproducibility{
			KBUILDBuildTimestamp: "2026-01-01T00:00:00Z",
			KBUILDBuildUser:      "gosd",
			KBUILDBuildHost:      "gosd-ci",
		},
	}
}

func testOverlay() kernelbuild.Overlay {
	return kernelbuild.Overlay{
		ConfigFragment: []byte("CONFIG_OVERLAY=y\n"),
		Patches: []kernelspec.Patch{
			{Name: "overlay.patch", Content: []byte("overlay patch\n")},
		},
	}
}

// fakeRunner is a containerRunner test double. By default it simulates a
// successful build by writing spec's expected output filenames into the
// mount at /out; runFn can be set to override that behavior (e.g. to
// simulate a RunFailedError).
type fakeRunner struct {
	calls []container.RunSpec
	runFn func(spec container.RunSpec) error
}

func newSucceedingRunner(spec kernelspec.KernelSpec) *fakeRunner {
	return &fakeRunner{
		runFn: func(runSpec container.RunSpec) error {
			return writeFakeOutputs(runSpec, spec)
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

func writeFakeOutputs(runSpec container.RunSpec, spec kernelspec.KernelSpec) error {
	outDir := mountHostPath(runSpec, "/out")
	if outDir == "" {
		return errors.New("no /out mount found in RunSpec")
	}
	files := map[string]string{
		spec.KernelFilename: "fake kernel image\n",
		"kernel.config":     "# fake generated config\n",
	}
	if spec.DTB != nil {
		files[spec.DTB.Filename] = "fake dtb\n"
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(outDir, name), []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func TestBuild_RunsContainerAndCollectsFlatOutput(t *testing.T) {
	spec := testSpec()
	rt := newSucceedingRunner(spec)
	flatDir := t.TempDir()

	result, err := kernelbuild.Build(context.Background(), spec, testOverlay(), kernelbuild.Options{
		Runtime:  rt,
		CacheDir: t.TempDir(),
		Outputs:  kernelbuild.Outputs{FlatDir: flatDir},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if result.Skipped {
		t.Fatal("Skipped = true on a fresh cache, want false")
	}
	if len(rt.calls) != 1 {
		t.Fatalf("container was run %d times, want 1", len(rt.calls))
	}

	for _, name := range []string{spec.KernelFilename, spec.DTB.Filename, "kernel.config", "source.json"} {
		if _, err := os.Stat(filepath.Join(flatDir, name)); err != nil {
			t.Errorf("flat output missing %s: %v", name, err)
		}
	}
}

// The /work bind mount must come from under the cache dir, never os.TempDir():
// macOS's default temp dir (/var/folders/…) isn't shared with Docker Desktop's
// VM, so a mount from there is silently empty in the container (gosd-0p21).
func TestBuild_WorkDirLivesUnderCacheDir(t *testing.T) {
	spec := testSpec()
	rt := newSucceedingRunner(spec)
	cacheDir := t.TempDir()

	if _, err := kernelbuild.Build(context.Background(), spec, testOverlay(), kernelbuild.Options{
		Runtime: rt, CacheDir: cacheDir,
	}); err != nil {
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
	overlay := testOverlay()

	if _, err := kernelbuild.Build(context.Background(), spec, overlay, kernelbuild.Options{
		Runtime: rt, CacheDir: cacheDir,
	}); err != nil {
		t.Fatalf("first Build: %v", err)
	}
	if len(rt.calls) != 1 {
		t.Fatalf("after first build, container ran %d times, want 1", len(rt.calls))
	}

	flatDir := t.TempDir()
	result, err := kernelbuild.Build(context.Background(), spec, overlay, kernelbuild.Options{
		Runtime: rt, CacheDir: cacheDir, Outputs: kernelbuild.Outputs{FlatDir: flatDir},
	})
	if err != nil {
		t.Fatalf("second Build: %v", err)
	}
	if !result.Skipped {
		t.Error("Skipped = false on repeat build with identical inputs, want true")
	}
	if len(rt.calls) != 1 {
		t.Fatalf("container ran %d times after cache-hit build, want still 1 (no re-run)", len(rt.calls))
	}
	if _, err := os.Stat(filepath.Join(flatDir, spec.KernelFilename)); err != nil {
		t.Errorf("cache-hit build did not collect flat output: %v", err)
	}
}

func TestBuild_CacheMissesOnChangedInput(t *testing.T) {
	base := testSpec()
	baseOverlay := testOverlay()

	cases := map[string]struct {
		spec    kernelspec.KernelSpec
		overlay kernelbuild.Overlay
		image   string
	}{
		"ref changed": {spec: withRef(base, "0000000000000000000000000000000000000000"), overlay: baseOverlay},
		"fragment byte changed": {
			spec:    withFragment(base, []byte("CONFIG_FOO=y\nCONFIG_EXTRA=y\n")),
			overlay: baseOverlay,
		},
		"overlay changed": {
			spec: base,
			overlay: kernelbuild.Overlay{
				ConfigFragment: []byte("CONFIG_DIFFERENT=y\n"),
			},
		},
	}

	baseKey := buildAndGetKey(t, base, baseOverlay, "")
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			key := buildAndGetKey(t, c.spec, c.overlay, c.image)
			if key == baseKey {
				t.Errorf("cache key unchanged after %s", name)
			}
		})
	}
}

func TestBuild_CacheMissesOnImageChange(t *testing.T) {
	spec := testSpec()
	overlay := testOverlay()
	cacheDir := t.TempDir()

	rt1 := newSucceedingRunner(spec)
	r1, err := kernelbuild.Build(context.Background(), spec, overlay, kernelbuild.Options{
		Runtime: rt1, CacheDir: cacheDir, Image: "image-a",
	})
	if err != nil {
		t.Fatalf("build with image-a: %v", err)
	}

	rt2 := newSucceedingRunner(spec)
	r2, err := kernelbuild.Build(context.Background(), spec, overlay, kernelbuild.Options{
		Runtime: rt2, CacheDir: cacheDir, Image: "image-b",
	})
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

func buildAndGetKey(t *testing.T, spec kernelspec.KernelSpec, overlay kernelbuild.Overlay, image string) string {
	t.Helper()
	rt := newSucceedingRunner(spec)
	result, err := kernelbuild.Build(context.Background(), spec, overlay, kernelbuild.Options{
		Runtime: rt, CacheDir: t.TempDir(), Image: image,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return result.CacheKey
}

func withRef(spec kernelspec.KernelSpec, ref string) kernelspec.KernelSpec {
	spec.Source.Ref = ref
	return spec
}

func withFragment(spec kernelspec.KernelSpec, fragment []byte) kernelspec.KernelSpec {
	spec.ConfigFragment = fragment
	return spec
}

func TestBuild_InterruptedBuildLeavesNoCacheEntry(t *testing.T) {
	spec := testSpec()
	cacheDir := t.TempDir()
	rt := &fakeRunner{runFn: func(container.RunSpec) error {
		return errors.New("simulated container failure")
	}}

	_, err := kernelbuild.Build(context.Background(), spec, kernelbuild.Overlay{}, kernelbuild.Options{
		Runtime: rt, CacheDir: cacheDir,
	})
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

func TestBuild_RequiredYFailureNamesTheMissingSymbol(t *testing.T) {
	spec := testSpec()
	rt := &fakeRunner{runFn: func(runSpec container.RunSpec) error {
		return &container.RunFailedError{
			Runtime:  "docker",
			Image:    runSpec.Image,
			ExitCode: 1,
			StderrTail: "==> Asserting required options survived olddefconfig\n" +
				"FATAL: CONFIG_BAR=y did not survive olddefconfig\n",
		}
	}}

	_, err := kernelbuild.Build(context.Background(), spec, kernelbuild.Overlay{}, kernelbuild.Options{
		Runtime: rt, CacheDir: t.TempDir(),
	})
	if err == nil {
		t.Fatal("Build succeeded despite a RequiredY failure")
	}
	if !strings.Contains(err.Error(), "CONFIG_BAR") {
		t.Errorf("error %q does not name the missing symbol CONFIG_BAR", err.Error())
	}
}

func TestBuild_MissingOutputIsAnError(t *testing.T) {
	spec := testSpec()
	rt := &fakeRunner{runFn: func(runSpec container.RunSpec) error {
		// Simulate a script that "succeeds" but forgets to copy the DTB.
		outDir := mountHostPath(runSpec, "/out")
		_ = os.WriteFile(filepath.Join(outDir, spec.KernelFilename), []byte("kernel\n"), 0o644)
		_ = os.WriteFile(filepath.Join(outDir, "kernel.config"), []byte("config\n"), 0o644)
		return nil
	}}

	_, err := kernelbuild.Build(context.Background(), spec, kernelbuild.Overlay{}, kernelbuild.Options{
		Runtime: rt, CacheDir: t.TempDir(),
	})
	if err == nil {
		t.Fatal("Build succeeded despite a missing expected output file")
	}
}

func TestBuild_RequiresRuntime(t *testing.T) {
	_, err := kernelbuild.Build(context.Background(), testSpec(), kernelbuild.Overlay{}, kernelbuild.Options{})
	if err == nil {
		t.Fatal("Build succeeded with a nil Runtime")
	}
}
