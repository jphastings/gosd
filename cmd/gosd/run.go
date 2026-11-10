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
	"github.com/jphastings/gosd/internal/configtree"
	"github.com/jphastings/gosd/internal/diskfmt"
	"github.com/jphastings/gosd/internal/image"
	"github.com/jphastings/gosd/internal/kernelparam"
	"github.com/jphastings/gosd/internal/pipeline"
	"github.com/jphastings/gosd/internal/qemurun"
)

// qemuVirtBoardName is the fixed board `gosd run` always builds for. It's
// not a flag: qemu-virt is the only board gosd knows how to boot itself
// (see internal/boards/qemuvirt and CLAUDE.md's "qemu-virt board" locked
// decision), so there's nothing for a --board flag to select between here.
const qemuVirtBoardName = "qemu-virt"

var (
	runBuildConfigFile string

	runPort         int
	runMemoryMiB    int
	runQemuArgs     []string
	runKeep         bool
	runDisplay      bool
	runConfigDir    string
	runArtifactsDir string
	runGosdInitSrc  string
	runBootSize     string
	runDataFlush    bool
	runIngress      []string
	runDataSize     string
	runLabelPrefix  string
	runKernelParams []string
)

func newRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run [path-to-main-package]",
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
		Args: cobra.MaximumNArgs(1),
		RunE: runRun,
	}

	basics := newFlagSection("Flags")
	basics.StringVar(&runBuildConfigFile, "build-config", "",
		"TOML build config, as used by gosd build. gosd docs build-config")
	basics.IntVar(&runPort, "port", qemurun.DefaultPort,
		"port on this machine forwarded to the app's port 80")
	basics.IntVar(&runMemoryMiB, "memory", qemurun.DefaultMemoryMiB,
		"virtual machine RAM, in MiB")
	basics.BoolVar(&runKeep, "keep", false,
		"keep the built image after qemu exits")
	basics.BoolVar(&runDisplay, "display", false,
		"show the virtual machine's screen in a window")
	basics.StringVar(&runConfigDir, "boot-config-dir", "",
		"directory of default device settings, one value per file. gosd docs config")
	basics.StringVar(&runDataSize, "data-size", defaultDataSize,
		"size of the writable /data partition (eg. 2GiB), or 'expand' to fill the card; 0 for none")

	runtimeFlags := newFlagSection("Runtime")
	runtimeFlags.BoolVar(&runDataFlush, "data-flush", false,
		"write files on FAT volumes to the card on close at runtime; slower, and not as safe as fsync. gosd docs runtime")
	runtimeFlags.StringArrayVar(&runKernelParams, "kernel-param", nil,
		"extra kernel command-line parameter (repeatable)")

	ux := newFlagSection("User Experience")
	ux.StringVar(&runLabelPrefix, "label-prefix", "",
		"prefix of the card's partition labels (default: the app's name)")
	ux.StringArrayVar(&runIngress, "ingress", nil,
		fmt.Sprintf("expose your app's HTTP service to the internet using %s (repeatable). gosd docs ingress", strings.Join(ingressAgentNames(), " or ")))

	advanced := newFlagSection("Advanced")
	advanced.StringArrayVar(&runQemuArgs, "qemu-arg", nil,
		"extra qemu-system-aarch64 argument (repeatable)")
	advanced.StringVar(&runArtifactsDir, "artifacts-dir", "",
		"directory of the qemu-virt kernel to use before downloading")
	advanced.StringVar(&runBootSize, "boot-size", defaultBootSize,
		"size of the boot partition")
	advanced.StringVar(&runGosdInitSrc, "gosd-init-src", os.Getenv("GOSD_INIT_SRC"),
		"build gosd-init, the program that boots your app, from this source directory")

	useFlagSections(cmd, basics, runtimeFlags, ux, advanced)

	return cmd
}

func runRun(cmd *cobra.Command, args []string) error {
	fileCfg, fileBaseDir, err := loadBuildConfig(runBuildConfigFile)
	if err != nil {
		return err
	}
	applyFileValues(cmd.Flags(), fileCfg, fileBaseDir, runFileKeys())

	pkgPath, err := resolveMainOperand(args, fileCfg, fileBaseDir, "gosd run")
	if err != nil {
		return err
	}
	if err := validatePkgPath(pkgPath); err != nil {
		return err
	}

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

	extraKernelParams, err := kernelparam.Parse(runKernelParams)
	if err != nil {
		return err
	}
	if err := validateIngress([]boards.Board{b}, ingressSelected); err != nil {
		return err
	}

	// gosd run has no --data-filesystem flag of its own (it's a fast
	// inner-loop qemu-virt smoke test, not a shipping image), so its data
	// partition stays FAT32-only, same as before --data-filesystem existed.
	dataSizeBytes, dataExpand, err := parseDataSize(runDataSize, diskfmt.FAT32)
	if err != nil {
		return err
	}

	if err := validateIngressDataPartition(ingressSelected, dataSizeBytes, dataExpand); err != nil {
		return err
	}

	appName, err := deriveAppName(pkgPath)
	if err != nil {
		return err
	}

	labels, err := resolveLabels(runLabelPrefix, cmd.Flags().Changed("label-prefix") || fileCfg.IsSet("label-prefix"), appName)
	if err != nil {
		return err
	}
	printPartitionLabels(cmd, "gosd run", labels, dataSizeBytes > 0 || dataExpand)

	resolvedConfigDir, err := resolveConfigDir(pkgPath, runConfigDir)
	if err != nil {
		return err
	}

	tree, err := configtree.Build(resolvedConfigDir, ingressFeaturesFor(ingressSelected, b))
	if err != nil {
		return fmt.Errorf("assembling the %s configuration for qemu-virt failed: %w", configtree.Dir, err)
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

	appOpts := build.AppCompileOptions{Tags: boards.BuildTags(b)}
	appBinary := filepath.Join(workDir, appName)
	if err := build.CrossCompile(pkgPath, appBinary, appOpts, b.Arch()); err != nil {
		return fmt.Errorf("cross-compiling %s failed: %w", pkgPath, err)
	}

	appSignalsReady, err := build.ImportsPackage(pkgPath, appOpts, b.Arch(), readyImportPath)
	if err != nil {
		return fmt.Errorf("checking whether %s imports %s failed: %w", pkgPath, readyImportPath, err)
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
		Board:                  b,
		AppBinaryPath:          appBinary,
		InitBinaryPath:         initBinary,
		Config:                 boards.BuildConfig{Hostname: appName},
		ConfigTree:             tree,
		ArtifactsDir:           runArtifactsDir,
		CacheDir:               cacheDir,
		OutputPath:             imgPath,
		Labels:                 labels,
		DataSizeBytes:          dataSizeBytes,
		DataExpand:             dataExpand,
		DataFlush:              runDataFlush,
		BootSizeBytes:          bootSizeBytes,
		ExtraFiles:             extraFiles,
		ExtraExecutables:       extraExecutables,
		IngressCloudflared:     ingressSelected.Cloudflared,
		IngressTailscaleFunnel: ingressSelected.TailscaleFunnel,
		AppSignalsReady:        appSignalsReady,
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
		ImagePath:    imgPath,
		Port:         runPort,
		MemoryMiB:    runMemoryMiB,
		Display:      runDisplay,
		ExtraArgs:    runQemuArgs,
		KernelParams: extraKernelParams,
		Stdin:        cmd.InOrStdin(),
		Stdout:       cmd.OutOrStdout(),
		Stderr:       cmd.ErrOrStderr(),
	})
}
