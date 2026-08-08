package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/jphastings/gosd/internal/boards"
	"github.com/jphastings/gosd/internal/build"
	"github.com/jphastings/gosd/internal/image"
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
	runDisplay      bool
	runHostname     string
	runArtifactsDir string
	runGosdInitSrc  string
	runBootSize     string
	runDataFlush    bool
	runIngress      []string
	runDataSize     string
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
	cmd.Flags().BoolVar(&runDisplay, "display", false,
		"open qemu's default host display window (Cocoa on macOS, GTK on Linux) showing the guest's virtio-gpu output; serial console stays on this terminal")
	cmd.Flags().StringVar(&runHostname, "hostname", "",
		"device hostname (default: sanitized main package name); an explicit value is baked into gosd.toml and always wins, while the default is left commented out (same as gosd build --hostname)")
	cmd.Flags().StringVar(&runArtifactsDir, "artifacts-dir", "",
		"directory containing a local qemu-virt kernel (Image), checked before falling back to a pinned-URL/release download")
	cmd.Flags().StringVar(&runGosdInitSrc, "gosd-init-src", "",
		"directory containing gosd-init's main package source; overrides gosd's normal detection (dev checkout, then module cache) for unusual setups; also locates cmd/gosd-tsfunnel's source (its gosd-tsfunnel sibling directory) when --ingress tailscale-funnel is selected")
	cmd.Flags().StringVar(&runBootSize, "boot-size", defaultBootSize,
		"size of the FAT32 GOSD-BOOT partition (e.g. 512MiB, 2GiB); same flag as gosd build's --boot-size, useful for checking a large app still fits before a real build")
	cmd.Flags().BoolVar(&runDataFlush, "data-flush", false,
		"same flag as gosd build's --data-flush: mount GOSD-DATA, and any emmc/disk vfat volume, with the vfat \"flush\" option; default false (normal Linux writeback)")
	cmd.Flags().StringArrayVar(&runIngress, "ingress", nil,
		fmt.Sprintf("same flag as gosd build's --ingress: bake in a client that exposes an app's HTTP service to the public internet with zero app code (repeatable; supported values: %s) - qemu-virt is arm64, so this exercises the runtime path in CI", strings.Join(ingressAgentNames(), ", ")))
	cmd.Flags().StringVar(&runDataSize, "data-size", defaultDataSize,
		"same flag as gosd build's --data-size: size of the writable GOSD-DATA partition (e.g. 512MiB, 2GiB), or 'expand'; default 0 omits the partition - required by some --ingress agents (e.g. tailscale-funnel) that need to persist state across reboots")

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

	ingressSelected, err := parseIngressFlags(runIngress)
	if err != nil {
		return err
	}
	if err := validateIngress([]boards.Board{b}, ingressSelected); err != nil {
		return err
	}

	dataSizeBytes, dataExpand, err := parseDataSize(runDataSize)
	if err != nil {
		return err
	}

	if err := validateIngressDataPartition(ingressSelected, dataSizeBytes, dataExpand); err != nil {
		return err
	}

	appName := naming.Sanitize(filepath.Base(filepath.Clean(pkgPath)))
	hostnameExplicit := runHostname != ""
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
	if err := build.CrossCompile(pkgPath, appBinary, boards.BuildTag(b), b.Arch()); err != nil {
		return fmt.Errorf("cross-compiling %s failed: %w", pkgPath, err)
	}

	initBinary := filepath.Join(workDir, "gosd-init")
	if err := build.CrossCompileGosdInit(initBinary, runGosdInitSrc, b.Arch()); err != nil {
		return fmt.Errorf("cross-compiling gosd-init failed: %w", err)
	}

	var tsfunnelBinary string
	if ingressSelected.TailscaleFunnel {
		tsfunnelBinary = filepath.Join(workDir, "gosd-tsfunnel")
		if err := build.CrossCompileTsfunnel(tsfunnelBinary, runGosdInitSrc, b.Arch()); err != nil {
			return fmt.Errorf("cross-compiling gosd-tsfunnel failed: %w", err)
		}
	}

	cacheDir, err := artifactCacheDir()
	if err != nil {
		return err
	}

	bootSizeBytes, err := parseBootSize(runBootSize)
	if err != nil {
		return err
	}

	imgPath := filepath.Join(workDir, appName+"-qemu-virt.img")

	ctx := cmd.Context()

	var ingressGOARCHesNeeded []string
	if ingressSelected.Cloudflared {
		ingressGOARCHesNeeded = ingressGOARCHes([]boards.Board{b})
	}
	shared, err := resolveSharedContent(ctx, runArtifactsDir, ingressGOARCHesNeeded)
	if err != nil {
		return err
	}
	extraFiles, extraExecutables, err := openSharedContent(shared, b)
	if err != nil {
		return err
	}

	if tsfunnelBinary != "" {
		tf, err := openTsfunnelBinary(tsfunnelBinary)
		if err != nil {
			return err
		}
		if extraExecutables == nil {
			extraExecutables = make(map[string]io.Reader, 1)
		}
		extraExecutables[ingressTailscaleFunnelDest] = tf
	}

	opts := pipeline.Options{
		Board:          b,
		AppBinaryPath:  appBinary,
		InitBinaryPath: initBinary,
		Config: boards.BuildConfig{
			Hostname:         deviceHostname,
			HostnameExplicit: hostnameExplicit,
		},
		ArtifactsDir:           runArtifactsDir,
		CacheDir:               cacheDir,
		OutputPath:             imgPath,
		DataSizeBytes:          dataSizeBytes,
		DataExpand:             dataExpand,
		DataFlush:              runDataFlush,
		BootSizeBytes:          bootSizeBytes,
		ExtraFiles:             extraFiles,
		ExtraExecutables:       extraExecutables,
		IngressCloudflared:     ingressSelected.Cloudflared,
		IngressTailscaleFunnel: ingressSelected.TailscaleFunnel,
	}
	report, err := pipeline.Assemble(ctx, opts)
	if err != nil {
		if errors.Is(err, image.ErrBootPartitionFull) {
			return fmt.Errorf("building %s for qemu-virt failed: %w; pass a larger --boot-size than %s and rebuild", appName, err, humanizeBinaryBytes(bootSizeBytes))
		}
		return fmt.Errorf("building %s for qemu-virt failed: %w", appName, err)
	}
	printBootVolumeUsage(cmd, "qemu-virt", report)

	pruneDownloadCaches(cmd, runArtifactsDir)

	runCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	cmd.PrintErrf("gosd run: booting %s under qemu-system-aarch64 (Ctrl-C to stop)\n", imgPath)
	cmd.PrintErrf("gosd run: your app will be reachable at http://localhost:%d once gosd-init starts it and networking comes up\n", runPort)

	return qemurun.Run(runCtx, qemurun.Options{
		ImagePath: imgPath,
		Port:      runPort,
		MemoryMiB: runMemoryMiB,
		Display:   runDisplay,
		ExtraArgs: runQemuArgs,
		Stdin:     cmd.InOrStdin(),
		Stdout:    cmd.OutOrStdout(),
		Stderr:    cmd.ErrOrStderr(),
	})
}
