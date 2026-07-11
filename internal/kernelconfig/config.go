// Package kernelconfig parses gosd-kernel.toml, the developer-authored
// overlay config gosd build-kernel reads to layer custom Kconfig fragments
// and device-tree patches onto a board's kernelspec.KernelSpec.
//
// This is deliberately the *minimal* v1 subset: only per-board `fragment`
// and `patches` under `[kernel.<board-id>]`. Bean gosd-hkp7 grows this
// package in place to the full schema — strict unknown-key rejection, a
// top-level `[kernel]` table with `based-on`/`builder`, and `[[firmware]]`
// entries — once it lands; until then, an unrecognized key in the file is
// silently ignored (BurntSushi/toml's default decode behavior), not
// rejected.
package kernelconfig

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/BurntSushi/toml"

	"github.com/jphastings/gosd/internal/kernelbuild"
	"github.com/jphastings/gosd/internal/kernelspec"
)

// Config is the parsed gosd-kernel.toml, keyed by board ID.
type Config struct {
	Kernel map[string]BoardOverlay `toml:"kernel"`
}

// BoardOverlay is one [kernel.<board-id>] section: paths as written in the
// file, not yet resolved into file contents (see Config.Overlay).
type BoardOverlay struct {
	// Fragment is a path to a Kconfig fragment file, merged onto the
	// board's kernelspec.KernelSpec.ConfigFragment.
	Fragment string `toml:"fragment"`
	// Patches is a list of paths or globs to device-tree patch files,
	// applied (in the order they expand to, sorted) after every patch in
	// KernelSpec.DTSPatches.
	Patches []string `toml:"patches"`
}

// Parse parses gosd-kernel.toml's contents into a Config. Missing data
// (nil or empty, as when --config was not given and no default file
// exists) yields a zero Config and no error — every field is optional and a
// zero Config resolves to a no-op overlay for every board.
func Parse(data []byte) (Config, error) {
	if len(data) == 0 {
		return Config{}, nil
	}

	var cfg Config
	if _, err := toml.NewDecoder(bytes.NewReader(data)).Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("gosd-kernel.toml is not valid TOML: %w", err)
	}
	return cfg, nil
}

// Overlay resolves boardID's [kernel.<board-id>] section (if any) into a
// kernelbuild.Overlay, reading the fragment file and expanding+reading every
// patches entry as a glob. Relative paths resolve against baseDir — the
// directory gosd-kernel.toml itself lives in, matching how a developer
// editing that file would expect a relative path to behave. A board with no
// matching section, or a zero Config, resolves to the zero (no-op) Overlay.
func (c Config) Overlay(boardID, baseDir string) (kernelbuild.Overlay, error) {
	board, ok := c.Kernel[boardID]
	if !ok {
		return kernelbuild.Overlay{}, nil
	}

	var overlay kernelbuild.Overlay

	if board.Fragment != "" {
		path := resolvePath(baseDir, board.Fragment)
		data, err := os.ReadFile(path)
		if err != nil {
			return kernelbuild.Overlay{}, fmt.Errorf("gosd-kernel.toml [kernel.%s] fragment %q: %w", boardID, path, err)
		}
		overlay.ConfigFragment = data
	}

	for _, pattern := range board.Patches {
		resolved := resolvePath(baseDir, pattern)
		matches, err := filepath.Glob(resolved)
		if err != nil {
			return kernelbuild.Overlay{}, fmt.Errorf("gosd-kernel.toml [kernel.%s] patches entry %q is not a valid glob: %w", boardID, pattern, err)
		}
		if len(matches) == 0 {
			return kernelbuild.Overlay{}, fmt.Errorf("gosd-kernel.toml [kernel.%s] patches entry %q matched no files", boardID, pattern)
		}
		sort.Strings(matches)

		for _, m := range matches {
			data, err := os.ReadFile(m)
			if err != nil {
				return kernelbuild.Overlay{}, fmt.Errorf("gosd-kernel.toml [kernel.%s] patch %q: %w", boardID, m, err)
			}
			overlay.Patches = append(overlay.Patches, kernelspec.Patch{Name: filepath.Base(m), Content: data})
		}
	}

	return overlay, nil
}

func resolvePath(baseDir, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(baseDir, path)
}
