// Package kernelconfig parses gosd-kernel.toml, the developer-authored
// overlay config gosd build-kernel (and, for its [[firmware]] entries, gosd
// build) reads to layer custom Kconfig fragments, device-tree patches, and
// runtime firmware onto a board's kernelspec.KernelSpec.
//
// Parsing is strict (bean gosd-hkp7): unlike gosd.toml, this is a
// developer-authored build input, so any key Parse doesn't recognize -
// anywhere in the file - is an error naming the offending key, not silently
// ignored.
package kernelconfig

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/jphastings/gosd/internal/artifacts"
	"github.com/jphastings/gosd/internal/container"
	"github.com/jphastings/gosd/internal/kernelbuild"
	"github.com/jphastings/gosd/internal/kernelspec"
)

// Config is the parsed gosd-kernel.toml.
type Config struct {
	// BasedOn is [kernel].based-on: the artifacts release this developer
	// overlay was authored against. Empty means unset. Parse has already
	// checked it equals internal/artifacts.Version when non-empty - cross-
	// version kernel builds aren't supported in v1.
	BasedOn string
	// Builder is [kernel].builder: container.RuntimeDocker or
	// container.RuntimePodman, or empty for auto-detect. Parse has already
	// validated it. It is the default build-kernel --builder uses when the
	// flag itself is left unset; the flag wins when both are given.
	Builder string
	// Kernel holds each [kernel.<board-id>] section, keyed by board ID.
	Kernel map[string]BoardOverlay
	// Firmware holds every [[firmware]] entry: runtime blobs gosd build (not
	// build-kernel) fetches via internal/fetch and places under
	// /lib/firmware in the initramfs, alongside the board's own firmware.
	// They are never re-hosted, per the third-party-blob locked decision.
	Firmware []FirmwareFile
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

// FirmwareFile is one [[firmware]] entry: a runtime firmware blob fetched
// from url and verified against sha256, landing at dest under /lib/firmware
// in the initramfs.
type FirmwareFile struct {
	// URL is the upstream location to fetch the file from. Required.
	URL string
	// SHA256 is the expected digest of the fetched file (64 lowercase or
	// uppercase hex characters). Required.
	SHA256 string
	// Dest is the path under /lib/firmware the file lands at. Required;
	// must be a relative path that stays under /lib/firmware (no leading
	// "/", no ".." component).
	Dest string
}

// rawConfig mirrors gosd-kernel.toml's top level. Kernel is decoded into
// map[string]toml.Primitive - rather than a struct - because [kernel] mixes
// fixed scalar keys (based-on, builder) with dynamically-named board
// subtables ([kernel.<board-id>]) at the same level; each entry is decoded
// a second time, by hand, once Parse knows which kind of key it is. Doing
// so still keeps every key tracked by MetaData, so Parse's final
// md.Undecoded() check catches anything - at any nesting depth - that
// wasn't consumed by that second pass or by rawFirmware's ordinary struct
// decode.
type rawConfig struct {
	Kernel   map[string]toml.Primitive `toml:"kernel"`
	Firmware []rawFirmware             `toml:"firmware"`
}

type rawFirmware struct {
	URL    string `toml:"url"`
	SHA256 string `toml:"sha256"`
	Dest   string `toml:"dest"`
}

// Parse parses gosd-kernel.toml's contents into a Config. Missing data (nil
// or empty, as when --config was not given and no default file exists)
// yields a zero Config and no error - every field is optional and a zero
// Config resolves to a no-op overlay for every board with no extra
// firmware.
//
// Parsing is strict: an unrecognized key anywhere in the file is an error
// naming it, [[module]] is rejected outright (reserved for a future
// loadable-modules decision), and every [kernel.<board-id>]/[[firmware]]
// entry is validated (known board ID, well-formed sha256/dest, based-on
// matching this gosd's pinned artifacts release, a valid builder).
func Parse(data []byte) (Config, error) {
	if len(data) == 0 {
		return Config{}, nil
	}

	var raw rawConfig
	md, err := toml.NewDecoder(bytes.NewReader(data)).Decode(&raw)
	if err != nil {
		return Config{}, fmt.Errorf("gosd-kernel.toml is not valid TOML: %w", err)
	}

	if md.IsDefined("module") {
		return Config{}, fmt.Errorf(
			"gosd-kernel.toml: [[module]] is reserved for a future loadable-modules decision (bean gosd-2k9p); " +
				"kernels are currently monolithic — compile drivers in via a fragment instead")
	}

	cfg, err := decodeKernelSection(md, raw.Kernel)
	if err != nil {
		return Config{}, err
	}

	if cfg.BasedOn != "" && cfg.BasedOn != artifacts.Version {
		return Config{}, fmt.Errorf(
			"gosd-kernel.toml [kernel].based-on = %q does not match this gosd's pinned artifacts release %q; "+
				"cross-version kernel builds aren't supported yet — update based-on to %q, or use a gosd build "+
				"whose pinned artifacts release matches",
			cfg.BasedOn, artifacts.Version, artifacts.Version)
	}

	if cfg.Builder != "" && cfg.Builder != container.RuntimeDocker && cfg.Builder != container.RuntimePodman {
		return Config{}, fmt.Errorf(
			"gosd-kernel.toml [kernel].builder = %q is invalid; use %q or %q, or omit it to auto-detect",
			cfg.Builder, container.RuntimeDocker, container.RuntimePodman)
	}

	firmware, err := decodeFirmware(raw.Firmware)
	if err != nil {
		return Config{}, err
	}
	cfg.Firmware = firmware

	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		keys := make([]string, len(undecoded))
		for i, k := range undecoded {
			keys[i] = k.String()
		}
		sort.Strings(keys)
		return Config{}, fmt.Errorf("gosd-kernel.toml has an unknown key %q", keys[0])
	}

	return cfg, nil
}

// decodeKernelSection splits raw's entries into the [kernel] table's own
// based-on/builder scalars and its [kernel.<board-id>] subtables, in sorted
// key order for deterministic error messages.
func decodeKernelSection(md toml.MetaData, raw map[string]toml.Primitive) (Config, error) {
	cfg := Config{Kernel: make(map[string]BoardOverlay, len(raw))}

	keys := make([]string, 0, len(raw))
	for key := range raw {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		prim := raw[key]
		switch key {
		case "based-on":
			if err := md.PrimitiveDecode(prim, &cfg.BasedOn); err != nil {
				return Config{}, fmt.Errorf("gosd-kernel.toml [kernel].based-on must be a string: %w", err)
			}
		case "builder":
			if err := md.PrimitiveDecode(prim, &cfg.Builder); err != nil {
				return Config{}, fmt.Errorf("gosd-kernel.toml [kernel].builder must be a string: %w", err)
			}
		default:
			if !isKnownBoard(key) {
				return Config{}, fmt.Errorf(
					"gosd-kernel.toml [kernel.%s] is not a known board ID; known boards: %s",
					key, strings.Join(kernelspec.BoardIDs(), ", "))
			}
			var board BoardOverlay
			if err := md.PrimitiveDecode(prim, &board); err != nil {
				return Config{}, fmt.Errorf("gosd-kernel.toml [kernel.%s]: %w", key, err)
			}
			cfg.Kernel[key] = board
		}
	}

	return cfg, nil
}

func isKnownBoard(id string) bool {
	for _, known := range kernelspec.BoardIDs() {
		if known == id {
			return true
		}
	}
	return false
}

// decodeFirmware validates every [[firmware]] entry, in file order, so the
// first invalid entry is reported.
func decodeFirmware(raw []rawFirmware) ([]FirmwareFile, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	firmware := make([]FirmwareFile, 0, len(raw))
	for i, f := range raw {
		if f.URL == "" {
			return nil, fmt.Errorf("gosd-kernel.toml [[firmware]] entry %d: url is required", i)
		}
		if f.Dest == "" {
			return nil, fmt.Errorf("gosd-kernel.toml [[firmware]] entry %d (url %q): dest is required", i, f.URL)
		}
		if !validSHA256(f.SHA256) {
			return nil, fmt.Errorf(
				"gosd-kernel.toml [[firmware]] entry %d (dest %q): sha256 %q is not a valid 64-character hex digest",
				i, f.Dest, f.SHA256)
		}
		if err := validDest(f.Dest); err != nil {
			return nil, fmt.Errorf("gosd-kernel.toml [[firmware]] entry %d: dest %q is invalid: %w", i, f.Dest, err)
		}
		firmware = append(firmware, FirmwareFile(f))
	}
	return firmware, nil
}

func validSHA256(s string) bool {
	decoded, err := hex.DecodeString(s)
	return err == nil && len(decoded) == 32
}

// validDest rejects an absolute path or one that escapes /lib/firmware via
// "..", mirroring the same safety check internal/artifacts uses for tar
// entries extracted from a release tarball.
func validDest(dest string) error {
	if filepath.IsAbs(dest) {
		return fmt.Errorf("must be a relative path under /lib/firmware, not absolute")
	}
	clean := filepath.Clean(dest)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("must stay under /lib/firmware (no \"..\")")
	}
	return nil
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
