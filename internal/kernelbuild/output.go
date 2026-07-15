package kernelbuild

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jphastings/gosd/internal/artifacts"
	"github.com/jphastings/gosd/internal/fsutil"
	"github.com/jphastings/gosd/internal/kernelspec"
)

// Outputs selects where a successful (or cache-hit) Build's artifacts are
// written. Both may be set to get both layouts from a single build; the
// exact CLI flag(s) exposing this choice are bean gosd-abya's job.
type Outputs struct {
	// FlatDir, if non-empty, receives the board's artifact files (named
	// exactly like the KernelSpec's KernelFilename/DTB.Filename - which in
	// turn match the board's ArtifactRef.Names) plus the generated
	// kernel.config and source.json, flat - directly usable as
	// `gosd build --artifacts-dir`.
	FlatDir string
	// StagingDir, if non-empty, receives the same files under
	// StagingDir/<BoardID>/ - the layout build/artifacts/package.sh expects
	// (see that script's header comment).
	StagingDir string
}

// writeSourceJSON writes dir/source.json recording spec's upstream kernel
// repo/ref and the generated config's filename, in the
// internal/artifacts.ComponentSource shape build-artifacts.yml's
// hand-written source.json files already use.
func writeSourceJSON(dir string, spec kernelspec.KernelSpec) error {
	source := map[string]artifacts.ComponentSource{
		"kernel": {
			Repo:   spec.Source.Repo,
			Ref:    spec.Source.Ref,
			Config: generatedConfigName,
		},
	}
	data, err := json.MarshalIndent(source, "", "  ")
	if err != nil {
		return fmt.Errorf("kernelbuild: encoding %s: %w", sourceJSONName, err)
	}
	if err := os.WriteFile(filepath.Join(dir, sourceJSONName), data, 0o644); err != nil {
		return fmt.Errorf("kernelbuild: writing %s: %w", sourceJSONName, err)
	}
	return nil
}

// collectOutputs copies spec's build outputs out of the content-addressed
// cache entry at cacheDir into every layout outputs asks for.
func collectOutputs(cacheDir string, spec kernelspec.KernelSpec, outputs Outputs) error {
	names := allOutputNames(spec)

	if outputs.FlatDir != "" {
		if err := copyNamed(cacheDir, outputs.FlatDir, names); err != nil {
			return fmt.Errorf("kernelbuild: writing flat output to %s: %w", outputs.FlatDir, err)
		}
	}
	if outputs.StagingDir != "" {
		boardDir := filepath.Join(outputs.StagingDir, spec.BoardID)
		if err := copyNamed(cacheDir, boardDir, names); err != nil {
			return fmt.Errorf("kernelbuild: writing staging output to %s: %w", boardDir, err)
		}
	}
	return nil
}

func copyNamed(srcDir, dstDir string, names []string) error {
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}
	for _, name := range names {
		if err := fsutil.CopyFile(filepath.Join(srcDir, name), filepath.Join(dstDir, name)); err != nil {
			return err
		}
	}
	return nil
}
