package extbuild

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jphastings/gosd/internal/fsutil"
)

// writeSourceJSON writes dir/source.json recording every Spec.Sources
// provenance entry (the GPL carve-out epic gosd-oyhi locks): GoSD never
// re-hosts what a build script clones, only records where it came from. An
// empty Sources still produces a (empty-array) source.json, so its presence
// alone is part of cacheComplete's "this build finished" signal.
func writeSourceJSON(dir string, spec Spec) error {
	sources := spec.Sources
	if sources == nil {
		sources = []Source{}
	}
	data, err := json.MarshalIndent(sources, "", "  ")
	if err != nil {
		return fmt.Errorf("extbuild: encoding %s: %w", SourceJSONName, err)
	}
	if err := os.WriteFile(filepath.Join(dir, SourceJSONName), data, 0o644); err != nil {
		return fmt.Errorf("extbuild: writing %s: %w", SourceJSONName, err)
	}
	return nil
}

// collectOutput copies the built binary (and source.json) from the
// content-addressed cache entry at cacheDir into outputDir, when set, and
// returns the path a caller should open/read the binary from: the copy in
// outputDir if one was requested, otherwise the cache entry's own copy.
func collectOutput(cacheDir, name, outputDir string) (string, error) {
	if outputDir == "" {
		return filepath.Join(cacheDir, name), nil
	}

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", fmt.Errorf("extbuild: creating output dir %s: %w", outputDir, err)
	}
	for _, f := range []string{name, SourceJSONName} {
		if err := fsutil.CopyFile(filepath.Join(cacheDir, f), filepath.Join(outputDir, f)); err != nil {
			return "", fmt.Errorf("extbuild: writing output to %s: %w", outputDir, err)
		}
	}
	return filepath.Join(outputDir, name), nil
}
