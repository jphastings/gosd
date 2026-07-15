package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jphastings/gosd/internal/boards"
	"github.com/jphastings/gosd/internal/container"
	"github.com/jphastings/gosd/internal/extbuild"
	"github.com/jphastings/gosd/internal/extconfig"
	"github.com/jphastings/gosd/internal/fsutil"
)

// defaultExternalConfigFile is the recipe gosd build-external reads from
// when --config is not given.
const defaultExternalConfigFile = "./gosd-external.toml"

// defaultExternalOutputDir is the <arch>/<name>-keyed layout gosd
// build-external writes into when -o/--output is not given.
const defaultExternalOutputDir = "./gosd-externals/"

var (
	externalNames       []string
	externalArchTokens  []string
	externalOutput      string
	externalConfigPath  string
	externalBuilderPref string
)

func newBuildExternalCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "build-external",
		Short: "Cross-compile a companion binary from a gosd-external.toml recipe inside Docker/Podman",
		Long: `Cross-compile one or more companion binaries ("externals") declared in a
gosd-external.toml recipe inside a local Docker or Podman container, ready
to bundle into an image with 'gosd build --with-external'.

An external is any prebuilt static executable your app needs alongside it -
e.g. a static mpv for hardware-decoded video, supervised by your app over
its own IPC. You supply the build script; gosd-external.toml points at it
and declares which arch(es) to cross-compile for. This command drives your
local Docker or Podman daemon to run that script, verifies the result is a
fully static ELF binary matching the target arch (the initramfs has no
ld.so), and writes it to ./gosd-externals/<arch>/<name>, alongside a
source.json recording the upstream repos/refs/licenses the script pins -
GoSD never redistributes what it builds here; you compile it yourself from
sources you pin.

This command requires Docker or Podman to be installed and running.`,
		RunE: runBuildExternal,
	}

	cmd.Flags().StringVar(&externalConfigPath, "config", defaultExternalConfigFile,
		"gosd-external.toml recipe to build from")
	cmd.Flags().StringArrayVar(&externalNames, "name", nil,
		"external to build (repeatable); omit to build every [external.<name>] recipe in --config")
	cmd.Flags().StringArrayVar(&externalArchTokens, "arch", nil,
		fmt.Sprintf("arch to build (repeatable): %s; omit to build every arch each selected recipe declares", strings.Join(sortedArchTokens(), ", ")))
	cmd.Flags().StringVarP(&externalOutput, "output", "o", defaultExternalOutputDir,
		"output directory, written as <output>/<arch>/<name>")
	cmd.Flags().StringVar(&externalBuilderPref, "builder", "",
		fmt.Sprintf("container runtime to use: %s or %s (default: the recipe's own [external.<name>].builder, or auto-detect if that's unset too)", container.RuntimeDocker, container.RuntimePodman))

	return cmd
}

func runBuildExternal(cmd *cobra.Command, _ []string) error {
	if err := validateBuilderPref(externalBuilderPref); err != nil {
		return err
	}

	cfg, cfgDir, err := loadExternalConfig(externalConfigPath)
	if err != nil {
		return err
	}

	selected, err := resolveExternals(cfg, externalNames)
	if err != nil {
		return err
	}

	archFilter, err := resolveArchTokens(externalArchTokens)
	if err != nil {
		return err
	}

	ctx := cmd.Context()
	outcomes, err := buildExternals(ctx, selected, cfgDir, archFilter, externalBuilderPref,
		detectExternalRuntime, extbuild.Build, externalOutput, cmd.OutOrStdout(), cmd.ErrOrStderr())
	if err != nil {
		return err
	}

	printExternalBuildSummary(cmd, outcomes, externalOutput)
	return nil
}

// loadExternalConfig reads and parses path. Unlike gosd build-kernel's
// optional developer overlay, a gosd-external.toml recipe IS the entire
// input this command acts on - there's nothing meaningful to build without
// one - so a missing file is an actionable error, not a silent no-op.
func loadExternalConfig(path string) (extconfig.Config, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return extconfig.Config{}, "", fmt.Errorf(
			"reading --config %s failed: %w; gosd build-external needs a gosd-external.toml recipe declaring at least one [external.<name>] to build", path, err)
	}

	cfg, err := extconfig.Parse(data)
	if err != nil {
		return extconfig.Config{}, "", fmt.Errorf("%s: %w", path, err)
	}
	if len(cfg.Externals) == 0 {
		return extconfig.Config{}, "", fmt.Errorf("%s declares no [external.<name>] recipes; add at least one", path)
	}
	return cfg, filepath.Dir(path), nil
}

// resolveExternals selects which of cfg's recipes to build: every recipe,
// in sorted name order, when names is empty; otherwise exactly the named
// ones (deduped, order-preserving), erroring on any name cfg doesn't have.
func resolveExternals(cfg extconfig.Config, names []string) ([]extconfig.External, error) {
	if len(names) == 0 {
		all := make([]extconfig.External, 0, len(cfg.Externals))
		for _, ext := range cfg.Externals {
			all = append(all, ext)
		}
		sort.Slice(all, func(i, j int) bool { return all[i].Name < all[j].Name })
		return all, nil
	}

	seen := make(map[string]bool, len(names))
	selected := make([]extconfig.External, 0, len(names))
	for _, name := range names {
		if seen[name] {
			continue
		}
		seen[name] = true

		ext, ok := cfg.Externals[name]
		if !ok {
			return nil, fmt.Errorf("--name %q is not declared in the config; known externals: %s", name, strings.Join(externalConfigNames(cfg), ", "))
		}
		selected = append(selected, ext)
	}
	return selected, nil
}

// resolveArchTokens validates and resolves repeatable --arch values against
// boards.KnownArches. A nil/empty tokens means no filter at all ("every
// arch each selected recipe declares"), distinguished by buildExternals
// from an explicit, validated filter list.
func resolveArchTokens(tokens []string) ([]boards.Arch, error) {
	if len(tokens) == 0 {
		return nil, nil
	}

	seen := make(map[string]bool, len(tokens))
	arches := make([]boards.Arch, 0, len(tokens))
	for _, tok := range tokens {
		if seen[tok] {
			continue
		}
		seen[tok] = true

		arch, ok := boards.KnownArches[tok]
		if !ok {
			return nil, fmt.Errorf("--arch %q is not known; known arches: %s", tok, strings.Join(sortedArchTokens(), ", "))
		}
		arches = append(arches, arch)
	}
	return arches, nil
}

// detectExternalRuntime is the production seam for buildExternals' detect
// argument: it wraps container.Detect, naming this command in any
// Docker/Podman-missing error it returns.
func detectExternalRuntime(ctx context.Context, preferred string) (containerRuntime, error) {
	rt, err := container.Detect(ctx, "gosd build-external", preferred)
	if err != nil {
		return nil, err
	}
	return rt, nil
}

// detectRuntimeFunc resolves a container runtime for a given --builder-style
// preference. Production passes detectExternalRuntime; tests substitute a
// fake so this command's orchestration is exercised without Docker/Podman
// installed.
type detectRuntimeFunc func(ctx context.Context, preferred string) (containerRuntime, error)

// externalBuildFunc matches extbuild.Build's signature. Production callers
// pass extbuild.Build directly; tests substitute a fake that writes
// expected output files without running a container.
type externalBuildFunc func(ctx context.Context, spec extbuild.Spec, opts extbuild.Options) (extbuild.Result, error)

// externalBuildOutcome reports what happened for one (name, arch) build.
type externalBuildOutcome struct {
	name    string
	arch    string
	skipped bool
	path    string
}

// buildExternals builds every arch of every selected recipe, in order,
// writing each result to outputDir/<arch>/<name> (plus a
// outputDir/<arch>/<name>.source.json provenance file - named per-external
// rather than extbuild's fixed "source.json" because multiple externals can
// share one arch's output directory, and a bare "source.json" would
// silently overwrite between them).
//
// The container runtime is resolved once per recipe (not once per build):
// gosd-external.toml lets each [external.<name>] pin its own builder, so
// two recipes in the same file can legitimately want different engines.
// Detect results are cached per distinct preference string to avoid a
// redundant `docker info`/`podman info` liveness check per arch.
func buildExternals(
	ctx context.Context,
	selected []extconfig.External,
	cfgDir string,
	archFilter []boards.Arch,
	builderPref string,
	detect detectRuntimeFunc,
	build externalBuildFunc,
	outputDir string,
	stdout, stderr io.Writer,
) ([]externalBuildOutcome, error) {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating output directory %s failed: %w", outputDir, err)
	}

	runtimes := make(map[string]containerRuntime)

	var outcomes []externalBuildOutcome
	for _, ext := range selected {
		arches, err := archesForExternal(ext, archFilter)
		if err != nil {
			return nil, err
		}

		script, err := ext.ReadScript(cfgDir)
		if err != nil {
			return nil, err
		}

		pref := effectiveExternalBuilderPref(builderPref, ext.Builder)
		rt, ok := runtimes[pref]
		if !ok {
			rt, err = detect(ctx, pref)
			if err != nil {
				return nil, err
			}
			runtimes[pref] = rt
		}

		for _, arch := range arches {
			spec := extbuild.Spec{
				Name:    ext.Name,
				Script:  script,
				Arch:    arch,
				Sources: toExtbuildSources(ext.Sources),
			}
			result, err := build(ctx, spec, extbuild.Options{
				Runtime: rt,
				Image:   ext.Image,
				Stdout:  stdout,
				Stderr:  stderr,
			})
			if err != nil {
				return nil, fmt.Errorf("building external %q for %s failed: %w", ext.Name, arch.Key(), err)
			}

			path, err := writeExternalOutput(outputDir, arch.Key(), ext.Name, result)
			if err != nil {
				return nil, err
			}

			outcomes = append(outcomes, externalBuildOutcome{name: ext.Name, arch: arch.Key(), skipped: result.Skipped, path: path})
		}
	}

	return outcomes, nil
}

// archesForExternal resolves which of ext's declared arches to build: all
// of them when filter is empty (--arch omitted), otherwise their
// intersection with filter. An empty intersection - filter given, but none
// of it matches anything ext declares - is an error naming ext, since
// selecting ext but filtering to arches it doesn't build is almost always a
// mistake (e.g. a typo, or the wrong --name/--arch pairing).
func archesForExternal(ext extconfig.External, filter []boards.Arch) ([]boards.Arch, error) {
	if len(filter) == 0 {
		return ext.Arch, nil
	}

	declared := make(map[string]bool, len(ext.Arch))
	for _, a := range ext.Arch {
		declared[a.Key()] = true
	}

	var matched []boards.Arch
	for _, a := range filter {
		if declared[a.Key()] {
			matched = append(matched, a)
		}
	}
	if len(matched) == 0 {
		return nil, fmt.Errorf(
			"external %q declares arch(es) %s, none of which match the requested --arch (%s)",
			ext.Name, strings.Join(archKeys(ext.Arch), ", "), strings.Join(archKeys(filter), ", "))
	}
	return matched, nil
}

// effectiveExternalBuilderPref resolves the container runtime preference
// for one recipe's build: the --builder flag wins whenever it's set;
// otherwise the recipe's own gosd-external.toml [external.<name>].builder
// (already validated by extconfig.Parse) is used; an empty result from both
// falls through to container.Detect's own auto-detection.
func effectiveExternalBuilderPref(flag, recipeBuilder string) string {
	if flag != "" {
		return flag
	}
	return recipeBuilder
}

func toExtbuildSources(sources []extconfig.Source) []extbuild.Source {
	if len(sources) == 0 {
		return nil
	}
	out := make([]extbuild.Source, len(sources))
	for i, s := range sources {
		out[i] = extbuild.Source{Name: s.Name, Repo: s.Repo, Ref: s.Ref, License: s.License}
	}
	return out
}

// writeExternalOutput copies result's binary and provenance out of
// extbuild's cache into outputDir/<archKey>/, returning the binary's final
// path (the one a developer hands to gosd build --with-external).
func writeExternalOutput(outputDir, archKey, name string, result extbuild.Result) (string, error) {
	destDir := filepath.Join(outputDir, archKey)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", fmt.Errorf("creating output directory %s failed: %w", destDir, err)
	}

	destBin := filepath.Join(destDir, name)
	if err := fsutil.CopyFile(result.OutputPath, destBin); err != nil {
		return "", fmt.Errorf("writing output %s failed: %w", destBin, err)
	}

	srcJSON := filepath.Join(result.CacheDir, extbuild.SourceJSONName)
	destJSON := filepath.Join(destDir, name+"."+extbuild.SourceJSONName)
	if err := fsutil.CopyFile(srcJSON, destJSON); err != nil {
		return "", fmt.Errorf("writing output %s failed: %w", destJSON, err)
	}

	return destBin, nil
}

func printExternalBuildSummary(cmd *cobra.Command, outcomes []externalBuildOutcome, outputDir string) {
	for _, o := range outcomes {
		status := "built"
		if o.skipped {
			status = "cache hit, not rebuilt"
		}
		cmd.PrintErrf("gosd build-external: %s (%s): %s\n", o.name, o.arch, status)
	}
	if len(outcomes) == 0 {
		cmd.PrintErrf("gosd build-external: done; nothing was built\n")
		return
	}
	cmd.PrintErrf("gosd build-external: done; use it with: gosd build --with-external %s\n", outcomes[0].path)
}

func sortedArchTokens() []string {
	tokens := make([]string, 0, len(boards.KnownArches))
	for tok := range boards.KnownArches {
		tokens = append(tokens, tok)
	}
	sort.Strings(tokens)
	return tokens
}

func externalConfigNames(cfg extconfig.Config) []string {
	names := make([]string, 0, len(cfg.Externals))
	for name := range cfg.Externals {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func archKeys(arches []boards.Arch) []string {
	keys := make([]string, len(arches))
	for i, a := range arches {
		keys[i] = a.Key()
	}
	return keys
}
