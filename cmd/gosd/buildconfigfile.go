package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/pflag"

	"github.com/jphastings/gosd/internal/buildconfig"
)

// defaultBuildConfigFile is stat'd in the working directory when
// --build-config isn't given, the same discovery rule as gosd-kernel.toml —
// deliberately no walk-up, so a build in a scratch subdirectory can't
// silently inherit inputs the developer can't see.
const defaultBuildConfigFile = "gosd-build.toml"

// loadBuildConfig resolves --build-config (or the default gosd-build.toml
// in the working directory, if present and explicit was empty) and parses
// it. No file found at all — the common case — is not an error: it returns
// the zero Config, which sets nothing. The returned base directory is the
// file's own directory made absolute: every relative path in the file
// resolves against it, so the file means the same thing however gosd is
// invoked.
func loadBuildConfig(explicit string) (buildconfig.Config, string, error) {
	path := explicit
	if path == "" {
		if _, err := os.Stat(defaultBuildConfigFile); err != nil {
			return buildconfig.Config{}, "", nil
		}
		path = defaultBuildConfigFile
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return buildconfig.Config{}, "", fmt.Errorf(
			"reading --build-config %s failed: %w; check the path, or drop the flag to use %s in the working directory when one exists",
			path, err, defaultBuildConfigFile)
	}

	cfg, err := buildconfig.Parse(data)
	if err != nil {
		if filepath.Base(path) != defaultBuildConfigFile {
			// Parse's messages name the canonical gosd-build.toml; point at
			// the file actually read when --build-config chose another name.
			return buildconfig.Config{}, "", fmt.Errorf("%s: %w", path, err)
		}
		return buildconfig.Config{}, "", err
	}

	baseDir, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return buildconfig.Config{}, "", fmt.Errorf("resolving %s's directory failed: %w", path, err)
	}
	return cfg, baseDir, nil
}

// fileKey ties one gosd-build.toml key to the flag it mirrors and the
// package-level flag variable it fills in.
type fileKey struct {
	flag  string
	key   string
	apply func(cfg buildconfig.Config, baseDir string)
}

// applyFileValues copies each file-set value into its flag variable unless
// that flag was given on the command line — the flag wins, per key. It runs
// after flag parsing, so a bool flag passed as --publish-catalog=false
// still beats a file `catalog = true` (Changed is what's checked, not the
// value).
func applyFileValues(flags *pflag.FlagSet, cfg buildconfig.Config, baseDir string, keys []fileKey) {
	for _, k := range keys {
		if cfg.IsSet(k.key) && !flags.Changed(k.flag) {
			k.apply(cfg, baseDir)
		}
	}
}

// buildFileKeys maps every gosd-build.toml key except app.main (the
// positional operand, see resolveMainOperand) onto gosd build's flag
// variables.
func buildFileKeys() []fileKey {
	return []fileKey{
		{"board", "board", func(c buildconfig.Config, _ string) { boardIDs = c.Board }},
		{"output", "output", func(c buildconfig.Config, d string) { output = buildconfig.ResolvePath(d, c.Output) }},
		{"label-prefix", "label-prefix", func(c buildconfig.Config, _ string) { labelPrefix = c.LabelPrefix }},
		{"ingress", "ingress", func(c buildconfig.Config, _ string) { ingressFlags = c.Ingress }},
		{"placeholder", "placeholder", func(c buildconfig.Config, _ string) { placeholders = c.Placeholder }},
		{"with-external", "with-external", func(c buildconfig.Config, d string) { withExternal = rebaseWithExternal(d, c.WithExternal) }},
		{"usb-gadget", "usb-gadget", func(c buildconfig.Config, _ string) { usbGadget = c.UsbGadget }},
		{"console-baud", "console-baud", func(c buildconfig.Config, _ string) { consoleBaud = c.ConsoleBaud }},
		{"artifacts-dir", "artifacts-dir", func(c buildconfig.Config, d string) { artifactsDir = buildconfig.ResolvePath(d, c.ArtifactsDir) }},
		{"gosd-init-src", "gosd-init-src", func(c buildconfig.Config, d string) { applyGosdInitSrc(&gosdInitSrc, c, d) }},
		{"app-version", "app.version", func(c buildconfig.Config, _ string) { appVersion = c.App.Version }},
		{"app-support-url", "app.support-url", func(c buildconfig.Config, _ string) { supportURL = c.App.SupportURL }},
		{"boot-size", "boot.size", func(c buildconfig.Config, _ string) { bootSize = c.Boot.Size }},
		{"boot-config-dir", "boot.config-dir", func(c buildconfig.Config, d string) { configDir = buildconfig.ResolvePath(d, c.Boot.ConfigDir) }},
		{"data-size", "data.size", func(c buildconfig.Config, _ string) { dataSize = c.Data.Size }},
		{"data-filesystem", "data.filesystem", func(c buildconfig.Config, _ string) { dataFilesystem = c.Data.Filesystem }},
		{"data-flush", "data.flush", func(c buildconfig.Config, _ string) { dataFlush = c.Data.Flush }},
		{"kernel-param", "kernel.param", func(c buildconfig.Config, _ string) { kernelParams = c.Kernel.Param }},
		{"kernel-config", "kernel.config", func(c buildconfig.Config, d string) { kernelCfgPath = buildconfig.ResolvePath(d, c.Kernel.Config) }},
		{"publish-catalog", "publish.catalog", func(c buildconfig.Config, _ string) { catalogFlag = c.Publish.Catalog }},
		{"publish-base-url", "publish.base-url", func(c buildconfig.Config, _ string) { publishBaseURL = c.Publish.BaseURL }},
	}
}

// runFileKeys covers the subset of keys whose flags gosd run mirrors; a
// build-only key under gosd run is deliberately ignored, the same way run
// has no --data-filesystem at all. Strict parsing still catches typos.
func runFileKeys() []fileKey {
	return []fileKey{
		{"label-prefix", "label-prefix", func(c buildconfig.Config, _ string) { runLabelPrefix = c.LabelPrefix }},
		{"ingress", "ingress", func(c buildconfig.Config, _ string) { runIngress = c.Ingress }},
		{"artifacts-dir", "artifacts-dir", func(c buildconfig.Config, d string) { runArtifactsDir = buildconfig.ResolvePath(d, c.ArtifactsDir) }},
		{"gosd-init-src", "gosd-init-src", func(c buildconfig.Config, d string) { applyGosdInitSrc(&runGosdInitSrc, c, d) }},
		{"boot-size", "boot.size", func(c buildconfig.Config, _ string) { runBootSize = c.Boot.Size }},
		{"boot-config-dir", "boot.config-dir", func(c buildconfig.Config, d string) { runConfigDir = buildconfig.ResolvePath(d, c.Boot.ConfigDir) }},
		{"data-size", "data.size", func(c buildconfig.Config, _ string) { runDataSize = c.Data.Size }},
		{"data-flush", "data.flush", func(c buildconfig.Config, _ string) { runDataFlush = c.Data.Flush }},
		{"kernel-param", "kernel.param", func(c buildconfig.Config, _ string) { runKernelParams = c.Kernel.Param }},
	}
}

// applyGosdInitSrc honors the gosd-init-src precedence flag > $GOSD_INIT_SRC
// > file: the env var is a per-machine installation pin (it's how package
// wrappers point at their bundled gosd-init), so a checked-in file that
// travels between machines must not silently defeat it. The flag tier is
// applyFileValues' Changed gate, as for every other key.
func applyGosdInitSrc(target *string, cfg buildconfig.Config, baseDir string) {
	if os.Getenv("GOSD_INIT_SRC") != "" {
		return
	}
	*target = buildconfig.ResolvePath(baseDir, cfg.GosdInitSrc)
}

// rebaseWithExternal resolves the local-file half of each
// "<path>[:<dest>]" entry against the config file's directory; the dest
// half is a path inside the image and is never rebased.
func rebaseWithExternal(baseDir string, entries []string) []string {
	out := make([]string, len(entries))
	for i, entry := range entries {
		path, dest := splitExternalFlag(entry)
		path = buildconfig.ResolvePath(baseDir, path)
		if dest == "" {
			out[i] = path
		} else {
			out[i] = path + ":" + dest
		}
	}
	return out
}

// resolveMainOperand picks the app's main-package path: the positional
// argument when given, else gosd-build.toml's [app].main. Only filesystem-
// relative forms are rebased onto the file's directory — an import path or
// absolute path passes through untouched, and anything else (say, a value
// starting with "-") is left for validatePkgPath to refuse, preserving its
// allow-list defence.
func resolveMainOperand(args []string, cfg buildconfig.Config, baseDir, command string) (string, error) {
	if len(args) > 0 {
		return args[0], nil
	}
	if main := cfg.App.Main; main != "" {
		if main == "." || main == ".." || strings.HasPrefix(main, "./") || strings.HasPrefix(main, "../") {
			return filepath.Join(baseDir, main), nil
		}
		return main, nil
	}
	return "", fmt.Errorf(
		"%s needs the path to your app's main package: pass it as an argument (e.g. '%s .' or '%s ./cmd/myapp'), or set main = \"./cmd/myapp\" under [app] in %s",
		command, command, command, defaultBuildConfigFile)
}
