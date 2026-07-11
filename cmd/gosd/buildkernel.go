package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jphastings/gosd/internal/boards"
	"github.com/jphastings/gosd/internal/container"
	"github.com/jphastings/gosd/internal/kernelbuild"
	"github.com/jphastings/gosd/internal/kernelconfig"
	"github.com/jphastings/gosd/internal/kernelspec"
)

// defaultKernelOutputDir is the flat --artifacts-dir-shaped layout gosd
// build-kernel writes into when -o/--output is not given.
const defaultKernelOutputDir = "./gosd-artifacts/"

// defaultKernelConfigFile is the developer overlay config gosd build-kernel
// looks for in the working directory when --config is not given.
const defaultKernelConfigFile = "gosd-kernel.toml"

var (
	kernelBoardIDs    []string
	kernelOutput      string
	kernelConfigPath  string
	kernelBuilderPref string
	kernelStagingDir  string
)

func newBuildKernelCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "build-kernel",
		Short: "Cross-compile a custom board kernel inside Docker/Podman (most developers don't need this)",
		Long: `Cross-compile a custom kernel for one or more boards inside a local Docker or
Podman container.

Most developers never need this command: gosd build already fetches pinned,
prebuilt kernel artifacts from a GoSD release, and that stays the fast
default path requiring no container runtime at all. Reach for
gosd build-kernel only when you need a driver GoSD's stock, trimmed kernels
cut (e.g. CONFIG_MEDIA_SUPPORT, USB DVB, a niche sensor) compiled in: declare
the extra Kconfig fragment and/or device-tree patches in gosd-kernel.toml,
and this command drives your local Docker or Podman daemon to build a kernel
with them applied, emitting a flat artifact directory that drops straight
into 'gosd build --artifacts-dir'.

This command requires Docker or Podman to be installed and running.`,
		RunE: runBuildKernel,
	}

	cmd.Flags().StringArrayVar(&kernelBoardIDs, "board", nil,
		fmt.Sprintf("board to build a kernel for (repeatable); omit to build every public board: %s", strings.Join(boards.IDs(), ", ")))
	cmd.Flags().StringVarP(&kernelOutput, "output", "o", defaultKernelOutputDir,
		"flat artifact output directory, ready for gosd build --artifacts-dir")
	cmd.Flags().StringVar(&kernelConfigPath, "config", "",
		fmt.Sprintf("developer overlay config (default: %s in the working directory, if present)", defaultKernelConfigFile))
	cmd.Flags().StringVar(&kernelBuilderPref, "builder", "",
		fmt.Sprintf("container runtime to use: %s or %s (default: gosd-kernel.toml's [kernel].builder, or auto-detect if that's unset too)", container.RuntimeDocker, container.RuntimePodman))
	cmd.Flags().StringVar(&kernelStagingDir, "staging", "",
		"also emit the staging/<board>/ layout build/artifacts/package.sh consumes (CI use)")

	return cmd
}

func runBuildKernel(cmd *cobra.Command, _ []string) error {
	if err := validateBuilderPref(kernelBuilderPref); err != nil {
		return err
	}

	selected, err := resolveBoards(kernelBoardIDs)
	if err != nil {
		return err
	}

	cfg, cfgDir, err := loadKernelConfig(kernelConfigPath)
	if err != nil {
		return err
	}

	ctx := cmd.Context()
	rt, err := container.Detect(ctx, effectiveBuilderPref(kernelBuilderPref, cfg))
	if err != nil {
		return err
	}

	outcomes, err := buildKernelsForBoards(ctx, selected, cfg, cfgDir, rt, kernelbuild.Build,
		kernelOutput, kernelStagingDir, cmd.OutOrStdout(), cmd.ErrOrStderr())
	if err != nil {
		return err
	}

	printKernelBuildSummary(cmd, outcomes, kernelOutput)
	return nil
}

// effectiveBuilderPref resolves the container runtime preference passed to
// container.Detect: the --builder flag wins whenever it's set; otherwise
// gosd-kernel.toml's [kernel].builder (already validated by
// kernelconfig.Parse) is used; an empty result from both falls through to
// container.Detect's own auto-detection.
func effectiveBuilderPref(flag string, cfg kernelconfig.Config) string {
	if flag != "" {
		return flag
	}
	return cfg.Builder
}

func validateBuilderPref(pref string) error {
	if pref == "" || pref == container.RuntimeDocker || pref == container.RuntimePodman {
		return nil
	}
	return fmt.Errorf("--builder %q is invalid; use %q or %q (or omit --builder to auto-detect)", pref, container.RuntimeDocker, container.RuntimePodman)
}

// loadKernelConfig resolves --config (or the default gosd-kernel.toml in the
// working directory, if present and explicit was empty) and parses it. No
// file found at all — the common case, since most developers never author a
// custom kernel overlay — is not an error: it returns the zero Config, which
// resolves to a no-op overlay for every board.
func loadKernelConfig(explicit string) (kernelconfig.Config, string, error) {
	path := explicit
	if path == "" {
		if _, err := os.Stat(defaultKernelConfigFile); err != nil {
			return kernelconfig.Config{}, "", nil
		}
		path = defaultKernelConfigFile
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return kernelconfig.Config{}, "", fmt.Errorf("reading --config %s failed: %w", path, err)
	}

	cfg, err := kernelconfig.Parse(data)
	if err != nil {
		return kernelconfig.Config{}, "", fmt.Errorf("%s: %w", path, err)
	}
	return cfg, filepath.Dir(path), nil
}

// containerRuntime is the subset of *container.Runtime that
// kernelbuild.Options.Runtime needs, defined locally (mirroring
// internal/kernelbuild's own unexported runner interface) so tests can
// inject a fake and exercise this command without a real Docker/Podman
// daemon. *container.Runtime satisfies it as-is.
type containerRuntime interface {
	Run(ctx context.Context, spec container.RunSpec) error
}

// kernelBuildFunc matches kernelbuild.Build's signature. Production callers
// pass kernelbuild.Build directly; tests substitute a fake that writes
// expected output files without running a container.
type kernelBuildFunc func(ctx context.Context, spec kernelspec.KernelSpec, overlay kernelbuild.Overlay, opts kernelbuild.Options) (kernelbuild.Result, error)

// kernelBuildOutcome reports what happened for one board's build.
type kernelBuildOutcome struct {
	board   string
	skipped bool
}

// buildKernelsForBoards builds selected's boards' kernels sequentially (each
// build already saturates every core, so there's no benefit to running them
// concurrently) via build. A board's failure aborts immediately with an
// error naming that board — boards after it in selected are never attempted
// (fail fast; CI runs one board per job anyway, so this only matters for a
// local multi-board gosd build-kernel invocation).
func buildKernelsForBoards(
	ctx context.Context,
	selected []boards.Board,
	cfg kernelconfig.Config,
	cfgDir string,
	rt containerRuntime,
	build kernelBuildFunc,
	outputDir, stagingDir string,
	stdout, stderr io.Writer,
) ([]kernelBuildOutcome, error) {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating output directory %s failed: %w", outputDir, err)
	}

	outcomes := make([]kernelBuildOutcome, 0, len(selected))
	for _, b := range selected {
		id := b.Name()

		spec, ok := kernelspec.Get(id)
		if !ok {
			return nil, fmt.Errorf("internal error: board %s has no registered kernelspec.KernelSpec", id)
		}

		overlay, err := cfg.Overlay(id, cfgDir)
		if err != nil {
			return nil, err
		}

		result, err := build(ctx, spec, overlay, kernelbuild.Options{
			Runtime: rt,
			Outputs: kernelbuild.Outputs{FlatDir: outputDir, StagingDir: stagingDir},
			Stdout:  stdout,
			Stderr:  stderr,
		})
		if err != nil {
			return nil, fmt.Errorf("building the kernel for %s failed: %w", id, err)
		}

		outcomes = append(outcomes, kernelBuildOutcome{board: id, skipped: result.Skipped})
	}

	return outcomes, nil
}

func printKernelBuildSummary(cmd *cobra.Command, outcomes []kernelBuildOutcome, outputDir string) {
	for _, o := range outcomes {
		status := "built"
		if o.skipped {
			status = "cache hit, not rebuilt"
		}
		cmd.PrintErrf("gosd build-kernel: %s: %s\n", o.board, status)
	}
	cmd.PrintErrf("gosd build-kernel: done; use it with e.g. gosd build --artifacts-dir %s <path-to-main-package>\n", outputDir)
}
