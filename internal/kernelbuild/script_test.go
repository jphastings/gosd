package kernelbuild_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jphastings/gosd/internal/container"
	"github.com/jphastings/gosd/internal/kernelbuild"
	"github.com/jphastings/gosd/internal/kernelspec"
)

// capturedScript builds spec+overlay against a fake runner that records the
// generated build.sh (and every support file) instead of pretending to run
// it, so script generation can be asserted on directly.
type capturedBuild struct {
	script         string
	env            map[string]string
	gosdPatchFiles []string
}

// captureBuild builds spec+overlay against a fake runner that, while the
// work dir bind-mounted at /work is still on disk (Build removes it once
// the container "run" returns), records the generated build.sh and the
// on-disk order of the GoSD patch files.
func captureBuild(t *testing.T, spec kernelspec.KernelSpec, overlay kernelbuild.Overlay) capturedBuild {
	t.Helper()
	var captured capturedBuild

	rt := &fakeRunner{runFn: func(runSpec container.RunSpec) error {
		captured.env = runSpec.Env
		workDir := ""
		for _, m := range runSpec.Mounts {
			if m.ContainerPath == "/work" {
				workDir = m.HostPath
			}
		}
		data, err := os.ReadFile(filepath.Join(workDir, "build.sh"))
		if err != nil {
			return err
		}
		captured.script = string(data)

		entries, err := os.ReadDir(filepath.Join(workDir, "patches", "gosd"))
		if err != nil {
			return err
		}
		for _, e := range entries {
			captured.gosdPatchFiles = append(captured.gosdPatchFiles, e.Name())
		}

		return writeFakeOutputs(runSpec, spec)
	}}

	if _, err := kernelbuild.Build(context.Background(), spec, overlay, kernelbuild.Options{
		Runtime: rt, CacheDir: t.TempDir(),
	}); err != nil {
		t.Fatalf("Build: %v", err)
	}
	return captured
}

func indexOf(t *testing.T, s, substr string) int {
	t.Helper()
	i := strings.Index(s, substr)
	if i == -1 {
		t.Fatalf("script does not contain %q:\n%s", substr, s)
	}
	return i
}

func TestScript_StepsPresentInOrder(t *testing.T) {
	c := captureBuild(t, testSpec(), kernelbuild.Overlay{})
	s := c.script

	clone := indexOf(t, s, "git -C \"$SRC_DIR\" init -q")
	gosdPatches := indexOf(t, s, "patches/gosd/*.patch")
	defconfig := indexOf(t, s, "make test_defconfig")
	gosdFragment := indexOf(t, s, "merge_config.sh -m .config /work/fragment.config")
	overlayPatches := indexOf(t, s, "patches/overlay/*.patch")
	overlayFragment := indexOf(t, s, "merge_config.sh -m .config /work/overlay-fragment.config")
	olddefconfig := indexOf(t, s, "make olddefconfig")
	assertions := indexOf(t, s, "Asserting required options survived olddefconfig")
	kernelBuild := indexOf(t, s, "make -j\"$(nproc)\" Image")
	dtbBuild := indexOf(t, s, "make -j\"$(nproc)\" test.dtb")
	copyOut := indexOf(t, s, "Copying outputs to /out")

	steps := []int{clone, gosdPatches, defconfig, gosdFragment, overlayPatches, overlayFragment, olddefconfig, assertions, kernelBuild, dtbBuild, copyOut}
	for i := 1; i < len(steps); i++ {
		if steps[i-1] >= steps[i] {
			t.Fatalf("build steps out of order: step %d (offset %d) is not before step %d (offset %d)\nscript:\n%s", i-1, steps[i-1], i, steps[i], s)
		}
	}
}

func TestScript_OverlayFragmentMergesAfterGoSDFragment(t *testing.T) {
	s := captureBuild(t, testSpec(), testOverlay()).script
	gosdFragment := indexOf(t, s, "merge_config.sh -m .config /work/fragment.config")
	overlayFragment := indexOf(t, s, "merge_config.sh -m .config /work/overlay-fragment.config")
	if overlayFragment < gosdFragment {
		t.Errorf("overlay fragment merge (offset %d) happens before GoSD fragment merge (offset %d)", overlayFragment, gosdFragment)
	}
}

func TestScript_OverlayPatchesApplyAfterGoSDPatches(t *testing.T) {
	s := captureBuild(t, testSpec(), testOverlay()).script
	gosdPatches := indexOf(t, s, "patches/gosd/*.patch")
	overlayPatches := indexOf(t, s, "patches/overlay/*.patch")
	if overlayPatches < gosdPatches {
		t.Errorf("overlay patch loop (offset %d) happens before GoSD patch loop (offset %d)", overlayPatches, gosdPatches)
	}
}

func TestScript_NoOverlayStillGuardsFragmentMerge(t *testing.T) {
	s := captureBuild(t, testSpec(), kernelbuild.Overlay{}).script
	if !strings.Contains(s, "if [ -s /work/overlay-fragment.config ]; then") {
		t.Error("script does not guard the overlay fragment merge behind a non-empty check")
	}
}

func TestScript_KBUILDPinsExportedAsContainerEnv(t *testing.T) {
	c := captureBuild(t, testSpec(), kernelbuild.Overlay{})
	want := map[string]string{
		"ARCH":                   "arm64",
		"CROSS_COMPILE":          "aarch64-linux-gnu-",
		"KBUILD_BUILD_TIMESTAMP": "2026-01-01T00:00:00Z",
		"KBUILD_BUILD_USER":      "gosd",
		"KBUILD_BUILD_HOST":      "gosd-ci",
	}
	for k, v := range want {
		if got := c.env[k]; got != v {
			t.Errorf("env[%q] = %q, want %q", k, got, v)
		}
	}
}

func TestScript_KBUILDPinsOmittedWhenEmpty(t *testing.T) {
	spec := testSpec()
	spec.Reproducibility = kernelspec.Reproducibility{}
	c := captureBuild(t, spec, kernelbuild.Overlay{})
	for _, k := range []string{"KBUILD_BUILD_TIMESTAMP", "KBUILD_BUILD_USER", "KBUILD_BUILD_HOST"} {
		if _, ok := c.env[k]; ok {
			t.Errorf("env[%q] set despite an empty Reproducibility pin", k)
		}
	}
}

func TestScript_RequiredYAndForbiddenYAssertionsIncluded(t *testing.T) {
	s := captureBuild(t, testSpec(), kernelbuild.Overlay{}).script

	for _, want := range []string{
		"CONFIG_FOO",
		"CONFIG_BAR",
		`echo "FATAL: ${opt}=y did not survive olddefconfig" >&2`,
		"CONFIG_BAZ",
		`echo "FATAL: ${opt} must not be set but is present" >&2`,
		"CONFIG_MODULES is not set",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("script missing expected assertion content %q", want)
		}
	}
}

func TestScript_TagRefUsesGitClone(t *testing.T) {
	spec := testSpec()
	spec.Source = kernelspec.Source{
		Repo:    "https://example.com/tagged.git",
		Ref:     "v1.2.3",
		RefKind: kernelspec.TagRef,
	}
	s := captureBuild(t, spec, kernelbuild.Overlay{}).script
	if !strings.Contains(s, `git clone --quiet --depth 1 --branch "v1.2.3" "https://example.com/tagged.git"`) {
		t.Errorf("tag-ref script does not shallow-clone by branch:\n%s", s)
	}
	if strings.Contains(s, "git fetch -q --depth 1 origin") {
		t.Errorf("tag-ref script unexpectedly uses the commit-ref fetch flow:\n%s", s)
	}
}

func TestScript_CommitRefUsesFetchAndCheckout(t *testing.T) {
	s := captureBuild(t, testSpec(), kernelbuild.Overlay{}).script
	if !strings.Contains(s, `fetch -q --depth 1 origin "deadbeefcafef00d"`) {
		t.Errorf("commit-ref script does not shallow-fetch the pinned commit:\n%s", s)
	}
	if !strings.Contains(s, "checkout -q FETCH_HEAD") {
		t.Errorf("commit-ref script does not check out FETCH_HEAD:\n%s", s)
	}
}

func TestScript_NoDTBSkipsDTBBuildAndCopy(t *testing.T) {
	spec := testSpec()
	spec.DTB = nil
	s := captureBuild(t, spec, kernelbuild.Overlay{}).script
	if strings.Contains(s, "test.dtb") {
		t.Errorf("script for a DTB-less board still references the DTB make target:\n%s", s)
	}
}

func TestScript_PatchFilesWrittenInOrder(t *testing.T) {
	c := captureBuild(t, testSpec(), testOverlay())
	if len(c.gosdPatchFiles) != 2 {
		t.Fatalf("patches/gosd has %d entries, want 2: %v", len(c.gosdPatchFiles), c.gosdPatchFiles)
	}
	if c.gosdPatchFiles[0] != "000-0001-a.patch" || c.gosdPatchFiles[1] != "001-0002-b.patch" {
		t.Errorf("patches/gosd entries = %v, want zero-padded-index-prefixed names preserving slice order", c.gosdPatchFiles)
	}
}
