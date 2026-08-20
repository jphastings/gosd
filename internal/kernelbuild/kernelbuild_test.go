package kernelbuild_test

import (
	"context"
	"errors"
	"fmt"
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
	for _, dtb := range spec.AllDTBs() {
		files[dtb.Filename] = "fake dtb\n"
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

// If the staging dir is deleted on the host mid-build (cache cleaner, macOS
// storage-pressure eviction - gosd-l4y9), the error must say that plainly
// instead of surfacing the container's confusing ENOENT.
func TestBuild_StagingDirVanishingMidBuildIsExplained(t *testing.T) {
	spec := testSpec()
	for name, runErr := range map[string]error{"run reports failure": errors.New("exit 1"), "run reports success": nil} {
		t.Run(name, func(t *testing.T) {
			rt := &fakeRunner{runFn: func(runSpec container.RunSpec) error {
				if err := os.RemoveAll(mountHostPath(runSpec, "/out")); err != nil {
					t.Fatal(err)
				}
				return runErr
			}}
			_, err := kernelbuild.Build(context.Background(), spec, testOverlay(), kernelbuild.Options{
				Runtime: rt, CacheDir: t.TempDir(),
			})
			if err == nil || !strings.Contains(err.Error(), "disappeared while the build was running") {
				t.Errorf("error = %v, want the vanished-staging explanation", err)
			}
		})
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

// TestBuild_CacheHitsDespiteRequiredYChange pins the documented, intentional
// exclusion (cache.go's cacheInputs doc comment): RequiredY/ForbiddenY are
// post-olddefconfig assertions, not build inputs, so fixing one alone must
// not force a rebuild of an otherwise-identical cached kernel.
func TestBuild_CacheHitsDespiteRequiredYChange(t *testing.T) {
	spec := testSpec()
	overlay := testOverlay()
	cacheDir := t.TempDir()
	rt := newSucceedingRunner(spec)

	if _, err := kernelbuild.Build(context.Background(), spec, overlay, kernelbuild.Options{
		Runtime: rt, CacheDir: cacheDir,
	}); err != nil {
		t.Fatalf("first Build: %v", err)
	}

	changed := spec
	changed.RequiredY = append([]string{"CONFIG_NEW=y"}, spec.RequiredY...)
	changed.ForbiddenY = append([]string{"CONFIG_NEWFORBIDDEN"}, spec.ForbiddenY...)

	result, err := kernelbuild.Build(context.Background(), changed, overlay, kernelbuild.Options{
		Runtime: rt, CacheDir: cacheDir,
	})
	if err != nil {
		t.Fatalf("second Build: %v", err)
	}
	if !result.Skipped {
		t.Error("Skipped = false after only RequiredY/ForbiddenY changed, want true (intentionally excluded from the cache key)")
	}
	if len(rt.calls) != 1 {
		t.Errorf("container ran %d times, want still 1 (RequiredY/ForbiddenY change must not force a rebuild)", len(rt.calls))
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
		// A newly-listed additional DTB must invalidate old cache entries,
		// which lack the new output file (pi-3b's 3B+ blob, bean gosd-oq0z).
		"additional DTB added": {spec: withAdditionalDTB(base), overlay: baseOverlay},
		// gosd-7jmj: each of these feeds buildScript (script.go) but was
		// missing from the cache key, so fixing one alone silently kept
		// re-serving the old, wrong cached kernel/DTB.
		"defconfig changed":      {spec: withDefconfig(base, "other_defconfig"), overlay: baseOverlay},
		"toolchain arch changed": {spec: withToolchainArch(base, "arm"), overlay: baseOverlay},
		"toolchain cross-compile changed": {
			spec: withToolchainCross(base, "arm-linux-gnueabihf-"), overlay: baseOverlay,
		},
		"kernel make target changed": {spec: withKernelMakeTarget(base, "zImage"), overlay: baseOverlay},
		"kernel source path changed": {
			spec: withKernelSourcePath(base, "arch/arm64/boot/other-image"), overlay: baseOverlay,
		},
		"DTB make target changed": {spec: withDTBMakeTarget(base, "other.dtb"), overlay: baseOverlay},
		"DTB source path changed": {
			spec: withDTBSourcePath(base, "arch/arm64/boot/dts/other.dtb"), overlay: baseOverlay,
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

func withAdditionalDTB(spec kernelspec.KernelSpec) kernelspec.KernelSpec {
	spec.AdditionalDTBs = []kernelspec.DTB{{
		MakeTarget: spec.DTB.MakeTarget,
		SourcePath: "arch/arm64/boot/dts/test-plus.dtb",
		Filename:   "test-board-plus.dtb",
	}}
	return spec
}

func withDefconfig(spec kernelspec.KernelSpec, defconfig string) kernelspec.KernelSpec {
	spec.Defconfig = defconfig
	return spec
}

func withToolchainArch(spec kernelspec.KernelSpec, arch string) kernelspec.KernelSpec {
	spec.Toolchain.KernelArch = arch
	return spec
}

func withToolchainCross(spec kernelspec.KernelSpec, cross string) kernelspec.KernelSpec {
	spec.Toolchain.CrossCompile = cross
	return spec
}

func withKernelMakeTarget(spec kernelspec.KernelSpec, target string) kernelspec.KernelSpec {
	spec.KernelMakeTarget = target
	return spec
}

func withKernelSourcePath(spec kernelspec.KernelSpec, path string) kernelspec.KernelSpec {
	spec.KernelSourcePath = path
	return spec
}

// withDTBMakeTarget and withDTBSourcePath copy spec.DTB so they don't
// mutate the shared base spec's DTB through its pointer.
func withDTBMakeTarget(spec kernelspec.KernelSpec, target string) kernelspec.KernelSpec {
	dtb := *spec.DTB
	dtb.MakeTarget = target
	spec.DTB = &dtb
	return spec
}

func withDTBSourcePath(spec kernelspec.KernelSpec, path string) kernelspec.KernelSpec {
	dtb := *spec.DTB
	dtb.SourcePath = path
	spec.DTB = &dtb
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

// TestBuild_BoundsCacheToTheMostRecentEntries exercises bean gosd-9o73 end
// to end: distinct cache keys (here, distinct Image values standing in for
// distinct board/kernelspec combinations) that never revisit an old key
// must not make the cache directory grow forever.
func TestBuild_BoundsCacheToTheMostRecentEntries(t *testing.T) {
	spec := testSpec()
	cacheDir := t.TempDir()

	const distinctBuilds = 10 // more than kernelbuild's keepBuildCacheEntries (8)
	var lastKey string
	for i := 0; i < distinctBuilds; i++ {
		rt := newSucceedingRunner(spec)
		result, err := kernelbuild.Build(context.Background(), spec, testOverlay(), kernelbuild.Options{
			Runtime: rt, CacheDir: cacheDir, Image: fmt.Sprintf("image-%d", i),
		})
		if err != nil {
			t.Fatalf("Build #%d: %v", i, err)
		}
		lastKey = result.CacheKey
	}

	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		t.Fatalf("reading cache dir: %v", err)
	}
	if len(entries) > 8 {
		t.Errorf("cache dir has %d entries after %d distinct builds, want at most 8 (kernelbuild's keep-last-N bound): %v", len(entries), distinctBuilds, entries)
	}

	found := false
	for _, e := range entries {
		if e.Name() == lastKey {
			found = true
		}
	}
	if !found {
		t.Errorf("the most recently built entry (%s) was pruned; entries = %v", lastKey, entries)
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
