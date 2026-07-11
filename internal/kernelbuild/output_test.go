package kernelbuild_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/jphastings/gosd/internal/artifacts"
	"github.com/jphastings/gosd/internal/kernelbuild"
)

func TestOutput_FlatDirHasExactBoardFilenamesPlusConfigAndSource(t *testing.T) {
	spec := testSpec()
	rt := newSucceedingRunner(spec)
	flatDir := t.TempDir()

	if _, err := kernelbuild.Build(context.Background(), spec, kernelbuild.Overlay{}, kernelbuild.Options{
		Runtime: rt, CacheDir: t.TempDir(), Outputs: kernelbuild.Outputs{FlatDir: flatDir},
	}); err != nil {
		t.Fatalf("Build: %v", err)
	}

	entries, err := os.ReadDir(flatDir)
	if err != nil {
		t.Fatalf("reading flat dir: %v", err)
	}
	got := make(map[string]bool)
	for _, e := range entries {
		got[e.Name()] = true
	}
	want := []string{spec.KernelFilename, spec.DTB.Filename, "kernel.config", "source.json"}
	for _, name := range want {
		if !got[name] {
			t.Errorf("flat dir missing %q; got %v", name, got)
		}
	}
	if len(got) != len(want) {
		t.Errorf("flat dir has %d entries %v, want exactly %v", len(got), got, want)
	}
}

func TestOutput_StagingDirUsesBoardIDSubdirectory(t *testing.T) {
	spec := testSpec()
	rt := newSucceedingRunner(spec)
	stagingDir := t.TempDir()

	if _, err := kernelbuild.Build(context.Background(), spec, kernelbuild.Overlay{}, kernelbuild.Options{
		Runtime: rt, CacheDir: t.TempDir(), Outputs: kernelbuild.Outputs{StagingDir: stagingDir},
	}); err != nil {
		t.Fatalf("Build: %v", err)
	}

	boardDir := filepath.Join(stagingDir, spec.BoardID)
	for _, name := range []string{spec.KernelFilename, spec.DTB.Filename, "kernel.config", "source.json"} {
		if _, err := os.Stat(filepath.Join(boardDir, name)); err != nil {
			t.Errorf("staging/%s missing %s: %v", spec.BoardID, name, err)
		}
	}
}

func TestOutput_BothFlatAndStagingRequested(t *testing.T) {
	spec := testSpec()
	rt := newSucceedingRunner(spec)
	flatDir := t.TempDir()
	stagingDir := t.TempDir()

	if _, err := kernelbuild.Build(context.Background(), spec, kernelbuild.Overlay{}, kernelbuild.Options{
		Runtime: rt, CacheDir: t.TempDir(),
		Outputs: kernelbuild.Outputs{FlatDir: flatDir, StagingDir: stagingDir},
	}); err != nil {
		t.Fatalf("Build: %v", err)
	}

	if _, err := os.Stat(filepath.Join(flatDir, spec.KernelFilename)); err != nil {
		t.Errorf("flat output missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(stagingDir, spec.BoardID, spec.KernelFilename)); err != nil {
		t.Errorf("staging output missing: %v", err)
	}
}

func TestOutput_SourceJSONContents(t *testing.T) {
	spec := testSpec()
	rt := newSucceedingRunner(spec)
	flatDir := t.TempDir()

	if _, err := kernelbuild.Build(context.Background(), spec, kernelbuild.Overlay{}, kernelbuild.Options{
		Runtime: rt, CacheDir: t.TempDir(), Outputs: kernelbuild.Outputs{FlatDir: flatDir},
	}); err != nil {
		t.Fatalf("Build: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(flatDir, "source.json"))
	if err != nil {
		t.Fatalf("reading source.json: %v", err)
	}
	var got map[string]artifacts.ComponentSource
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("parsing source.json: %v", err)
	}

	want := artifacts.ComponentSource{
		Repo:   spec.Source.Repo,
		Ref:    spec.Source.Ref,
		Config: "kernel.config",
	}
	kernel, ok := got["kernel"]
	if !ok {
		t.Fatalf("source.json has no \"kernel\" entry: %v", got)
	}
	if kernel != want {
		t.Errorf("source.json[\"kernel\"] = %+v, want %+v", kernel, want)
	}
}

// TestOutput_StagingLayoutMatchesPackageScriptExpectations documents the
// contract build/artifacts/package.sh relies on (see its header comment):
// one subdirectory per board, source.json present but excludable, every
// other regular file directly inside the board dir treated as a packaged
// artifact.
func TestOutput_StagingLayoutMatchesPackageScriptExpectations(t *testing.T) {
	spec := testSpec()
	rt := newSucceedingRunner(spec)
	stagingDir := t.TempDir()

	if _, err := kernelbuild.Build(context.Background(), spec, kernelbuild.Overlay{}, kernelbuild.Options{
		Runtime: rt, CacheDir: t.TempDir(), Outputs: kernelbuild.Outputs{StagingDir: stagingDir},
	}); err != nil {
		t.Fatalf("Build: %v", err)
	}

	boardDir := filepath.Join(stagingDir, spec.BoardID)
	entries, err := os.ReadDir(boardDir)
	if err != nil {
		t.Fatalf("reading %s: %v", boardDir, err)
	}
	for _, e := range entries {
		if e.IsDir() {
			t.Errorf("staging board dir has an unexpected subdirectory %q; package.sh only looks at files directly inside it", e.Name())
		}
	}
	if _, err := os.Stat(filepath.Join(boardDir, "source.json")); err != nil {
		t.Errorf("staging board dir missing source.json: %v", err)
	}
}
