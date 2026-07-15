// Package extconfig parses gosd-external.toml, the developer-authored
// recipe `gosd build-external` (bean gosd-x3o0) reads to describe how to
// cross-compile a companion userspace binary (an "external", e.g. a static
// mpv) inside a container - see internal/extbuild for the builder that
// consumes it.
//
// Parsing mirrors internal/kernelconfig's strictness idiom (bean gosd-hkp7):
// this is a developer-authored build input, not gosd.toml, so any key
// Parse doesn't recognize - anywhere in the file - is an error naming the
// offending key, not silently ignored.
package extconfig

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/jphastings/gosd/internal/boards"
	"github.com/jphastings/gosd/internal/container"
)

// Config is the parsed gosd-external.toml: every [external.<name>] section,
// keyed by name.
type Config struct {
	Externals map[string]External
}

// External is one [external.<name>] section, as written in the file -
// Script is not yet resolved into an absolute path (see ScriptPath/
// ReadScript) or read.
type External struct {
	// Name is the section key, e.g. "mpv" for [external.mpv].
	Name string
	// Script is the build script path exactly as written; a relative path
	// resolves against the directory gosd-external.toml itself lives in
	// (see ScriptPath, ReadScript).
	Script string
	// Arch is [external.<name>].arch, resolved from its recipe tokens
	// (e.g. "arm64", "arm-6") against boards.KnownArches. Always has at
	// least one entry - Parse rejects an empty or missing list.
	Arch []boards.Arch
	// Image overrides the container image the build runs inside. Empty
	// means the caller should default to the shared kernel-build image
	// (container.KernelBuildImage), so gosd build-external and gosd
	// build-kernel keep Docker's layer cache warm against each other.
	Image string
	// Builder is container.RuntimeDocker or container.RuntimePodman, or
	// empty for auto-detect. Parse has already validated it.
	Builder string
	// Sources is every [[external.<name>.source]] entry: provenance-only
	// records of what the script pins and clones. GoSD never clones or
	// verifies these itself - the build script does the actual pinned
	// cloning (the GPL carve-out locked in epic gosd-oyhi).
	Sources []Source
}

// Source is one [[external.<name>.source]] provenance entry.
type Source struct {
	Name    string
	Repo    string
	Ref     string
	License string
}

// rawConfig mirrors gosd-external.toml's top level: External is decoded
// into map[string]toml.Primitive - rather than a struct - because each
// [external.<name>] table is dynamically named. Every entry is decoded a
// second time, by hand, once Parse knows its name; doing so still keeps
// every key tracked by MetaData, so Parse's final md.Undecoded() check
// catches anything - at any nesting depth - that wasn't consumed by that
// second pass.
type rawConfig struct {
	External map[string]toml.Primitive `toml:"external"`
}

type rawExternal struct {
	Script  string      `toml:"script"`
	Arch    []string    `toml:"arch"`
	Image   string      `toml:"image"`
	Builder string      `toml:"builder"`
	Source  []rawSource `toml:"source"`
}

type rawSource struct {
	Name    string `toml:"name"`
	Repo    string `toml:"repo"`
	Ref     string `toml:"ref"`
	License string `toml:"license"`
}

// Parse parses gosd-external.toml's contents into a Config. Missing data
// (nil or empty, as when no gosd-external.toml exists) yields a zero Config
// and no error.
//
// Parsing is strict: an unrecognized key anywhere in the file is an error
// naming it, and every [external.<name>] entry is validated (a non-empty
// script, at least one known arch, a valid builder if set, and every
// [[source]] entry fully populated).
func Parse(data []byte) (Config, error) {
	if len(data) == 0 {
		return Config{}, nil
	}

	var raw rawConfig
	md, err := toml.NewDecoder(bytes.NewReader(data)).Decode(&raw)
	if err != nil {
		return Config{}, fmt.Errorf("gosd-external.toml is not valid TOML: %w", err)
	}

	cfg, err := decodeExternalSection(md, raw.External)
	if err != nil {
		return Config{}, err
	}

	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		keys := make([]string, len(undecoded))
		for i, k := range undecoded {
			keys[i] = k.String()
		}
		sort.Strings(keys)
		return Config{}, fmt.Errorf("gosd-external.toml has an unknown key %q", keys[0])
	}

	return cfg, nil
}

// decodeExternalSection decodes every [external.<name>] primitive in
// sorted key order, for deterministic error messages.
func decodeExternalSection(md toml.MetaData, raw map[string]toml.Primitive) (Config, error) {
	cfg := Config{Externals: make(map[string]External, len(raw))}

	names := make([]string, 0, len(raw))
	for name := range raw {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		var rext rawExternal
		if err := md.PrimitiveDecode(raw[name], &rext); err != nil {
			return Config{}, fmt.Errorf("gosd-external.toml [external.%s]: %w", name, err)
		}

		ext, err := toExternal(name, rext)
		if err != nil {
			return Config{}, err
		}
		cfg.Externals[name] = ext
	}

	return cfg, nil
}

func toExternal(name string, raw rawExternal) (External, error) {
	if raw.Script == "" {
		return External{}, fmt.Errorf("gosd-external.toml [external.%s]: script is required", name)
	}

	arches, err := toArches(name, raw.Arch)
	if err != nil {
		return External{}, err
	}

	if raw.Builder != "" && raw.Builder != container.RuntimeDocker && raw.Builder != container.RuntimePodman {
		return External{}, fmt.Errorf(
			"gosd-external.toml [external.%s].builder = %q is invalid; use %q or %q, or omit it to auto-detect",
			name, raw.Builder, container.RuntimeDocker, container.RuntimePodman)
	}

	sources, err := toSources(name, raw.Source)
	if err != nil {
		return External{}, err
	}

	return External{
		Name:    name,
		Script:  raw.Script,
		Arch:    arches,
		Image:   raw.Image,
		Builder: raw.Builder,
		Sources: sources,
	}, nil
}

func toArches(name string, tokens []string) ([]boards.Arch, error) {
	if len(tokens) == 0 {
		return nil, fmt.Errorf(`gosd-external.toml [external.%s]: arch is required (e.g. arch = ["arm64"])`, name)
	}

	arches := make([]boards.Arch, 0, len(tokens))
	for _, token := range tokens {
		arch, ok := boards.KnownArches[token]
		if !ok {
			return nil, fmt.Errorf(
				"gosd-external.toml [external.%s] arch %q is not known; known arches: %s",
				name, token, strings.Join(knownArchTokens(), ", "))
		}
		arches = append(arches, arch)
	}
	return arches, nil
}

// toSources validates every [[source]] entry is fully populated: a
// provenance record missing its repo, ref, or license defeats the point of
// recording it (the GPL carve-out epic gosd-oyhi locks - GoSD never
// re-hosts what a script clones, only records where it came from).
func toSources(name string, raw []rawSource) ([]Source, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	sources := make([]Source, 0, len(raw))
	for i, s := range raw {
		if s.Name == "" {
			return nil, fmt.Errorf("gosd-external.toml [external.%s] source entry %d: name is required", name, i)
		}
		if s.Repo == "" {
			return nil, fmt.Errorf("gosd-external.toml [external.%s] source %q: repo is required", name, s.Name)
		}
		if s.Ref == "" {
			return nil, fmt.Errorf("gosd-external.toml [external.%s] source %q: ref is required", name, s.Name)
		}
		if s.License == "" {
			return nil, fmt.Errorf("gosd-external.toml [external.%s] source %q: license is required", name, s.Name)
		}
		sources = append(sources, Source(s))
	}
	return sources, nil
}

// ScriptPath resolves e.Script against baseDir - the directory
// gosd-external.toml itself lives in - without reading it. A relative
// Script joins onto baseDir; an absolute one is returned unchanged.
func (e External) ScriptPath(baseDir string) string {
	if filepath.IsAbs(e.Script) {
		return e.Script
	}
	return filepath.Join(baseDir, e.Script)
}

// ReadScript resolves e.Script against baseDir (see ScriptPath) and reads
// its contents, mirroring how kernelconfig.Config.Overlay resolves and
// reads a board's fragment/patch files relative to the same TOML file.
func (e External) ReadScript(baseDir string) ([]byte, error) {
	path := e.ScriptPath(baseDir)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("gosd-external.toml [external.%s] script %q: %w", e.Name, path, err)
	}
	return data, nil
}

func knownArchTokens() []string {
	tokens := make([]string, 0, len(boards.KnownArches))
	for token := range boards.KnownArches {
		tokens = append(tokens, token)
	}
	sort.Strings(tokens)
	return tokens
}
