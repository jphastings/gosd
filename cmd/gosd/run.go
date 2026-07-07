package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/jphastings/gosd/internal/boards"
	"github.com/jphastings/gosd/internal/build"
	"github.com/jphastings/gosd/internal/naming"
	"github.com/jphastings/gosd/internal/pipeline"
	"github.com/jphastings/gosd/internal/qemurun"
)

// qemuVirtBoardName is the fixed board `gosd run` always builds for. It's
// not a flag: qemu-virt is the only board gosd knows how to boot itself
// (see internal/boards/qemuvirt and CLAUDE.md's "qemu-virt board" locked
// decision), so there's nothing for a --board flag to select between here.
const qemuVirtBoardName = "qemu-virt"

var (
	runPort         int
	runMemoryMiB    int
	runQemuArgs     []string
	runKeep         bool
	runHostname     string
	runArtifactsDir string
	runGosdInitSrc  string
)

func newRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run <path-to-main-package>",
		Short: "Cross-compile, build a qemu-virt image, and boot it under qemu-system-aarch64",
		Long: `Cross-compile, build a qemu-virt image, and boot it under qemu-system-aarch64.

This is the fast inner-loop way to see your app run through gosd-init's real
boot sequence - mounts, supervision, DHCP, mDNS - without a Pi or a Radxa on
your desk. It builds the same internal, CI-only qemu-virt board build
produces (never a target you'd ship to end users) into a temporary
location, boots it with serial console on stdio, and forwards the guest's
HTTP port 80 to a port on your machine.

Ctrl-C stops qemu and cleans up the temporary image; --keep leaves it in
place and prints its path instead.`,
		Args: cobra.ExactArgs(1),
		RunE: runRun,
	}

	cmd.Flags().IntVar(&runPort, "port", qemurun.DefaultPort,
		"host port forwarded to the guest's HTTP port 80")
	cmd.Flags().IntVar(&runMemoryMiB, "memory", qemurun.DefaultMemoryMiB,
		"guest RAM in MiB")
	cmd.Flags().StringArrayVar(&runQemuArgs, "qemu-arg", nil,
		"extra argument to pass through to qemu-system-aarch64 (repeatable)")
	cmd.Flags().BoolVar(&runKeep, "keep", false,
		"keep the built image and temp build directory after qemu exits, instead of deleting them")
	cmd.Flags().StringVar(&runHostname, "hostname", "",
		"device hostname (default: sanitized main package name)")
	cmd.Flags().StringVar(&runArtifactsDir, "artifacts-dir", "",
		"directory containing a local qemu-virt kernel (Image), checked before falling back to a pinned-URL/release download")
	cmd.Flags().StringVar(&runGosdInitSrc, "gosd-init-src", "",
		"directory containing gosd-init's main package source; overrides gosd's normal detection (dev checkout, then module cache) for unusual setups")

	return cmd
}

func runRun(cmd *cobra.Command, args []string) error {
	pkgPath := args[0]

	// Fail fast, before spending any time cross-compiling or assembling
	// an image, if there's nothing to boot the result with.
	if err := qemurun.CheckAvailable(); err != nil {
		return err
	}

	b, ok := boards.Find(qemuVirtBoardName)
	if !ok {
		return fmt.Errorf("internal error: the %s board is not registered", qemuVirtBoardName)
	}

	appName := naming.Sanitize(filepath.Base(filepath.Clean(pkgPath)))
	deviceHostname := runHostname
	if deviceHostname == "" {
		deviceHostname = appName
	}

	workDir, err := os.MkdirTemp("", "gosd-run-")
	if err != nil {
		return fmt.Errorf("creating a temp build directory failed: %w", err)
	}
	defer func() {
		if runKeep {
			cmd.PrintErrf("gosd run: kept build artifacts at %s\n", workDir)
			return
		}
		_ = os.RemoveAll(workDir)
	}()

	appBinary := filepath.Join(workDir, appName)
	if err := build.CrossCompile(pkgPath, appBinary, b.Arch()); err != nil {
		return fmt.Errorf("cross-compiling %s failed: %w", pkgPath, err)
	}

	initBinary := filepath.Join(workDir, "gosd-init")
	if err := build.CrossCompileGosdInit(initBinary, runGosdInitSrc, b.Arch()); err != nil {
		return fmt.Errorf("cross-compiling gosd-init failed: %w", err)
	}

	cacheDir, err := artifactCacheDir()
	if err != nil {
		return err
	}

	dataSizeBytes, err := parseDataSize(defaultDataSize)
	if err != nil {
		return err
	}

	imgPath := filepath.Join(workDir, appName+"-qemu-virt.img")

	ctx := cmd.Context()
	opts := pipeline.Options{
		Board:          b,
		AppBinaryPath:  appBinary,
		InitBinaryPath: initBinary,
		Config: boards.BuildConfig{
			Hostname: deviceHostname,
		},
		ArtifactsDir:  runArtifactsDir,
		CacheDir:      cacheDir,
		OutputPath:    imgPath,
		DataSizeBytes: dataSizeBytes,
	}
	if err := pipeline.Assemble(ctx, opts); err != nil {
		return fmt.Errorf("building %s for qemu-virt failed: %w", appName, err)
	}

	runCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	cmd.PrintErrf("gosd run: booting %s under qemu-system-aarch64 (Ctrl-C to stop)\n", imgPath)
	cmd.PrintErrf("gosd run: your app will be reachable at http://localhost:%d once gosd-init starts it and networking comes up\n", runPort)

	return qemurun.Run(runCtx, qemurun.Options{
		ImagePath: imgPath,
		Port:      runPort,
		MemoryMiB: runMemoryMiB,
		ExtraArgs: runQemuArgs,
		Stdin:     cmd.InOrStdin(),
		Stdout:    cmd.OutOrStdout(),
		Stderr:    cmd.ErrOrStderr(),
	})
}
