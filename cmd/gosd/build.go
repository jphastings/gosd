package main

import (
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/spf13/cobra"

	"github.com/jphastings/gosd/internal/boards"
	// Registers every board gosd ships, via boardset's init(). Dropping
	// this import still compiles: it produces a gosd that knows about zero
	// boards, where a bare `gosd build` builds nothing and every --board is
	// unknown.
	_ "github.com/jphastings/gosd/internal/boardset"
	"github.com/jphastings/gosd/internal/build"
	"github.com/jphastings/gosd/internal/catalog"
	"github.com/jphastings/gosd/internal/configtree"
	"github.com/jphastings/gosd/internal/diskfmt"
	"github.com/jphastings/gosd/internal/image"
	"github.com/jphastings/gosd/internal/inject"
	"github.com/jphastings/gosd/internal/naming"
	"github.com/jphastings/gosd/internal/pipeline"
)

var (
	boardIDs       []string
	output         string
	configDir      string
	artifactsDir   string
	gosdInitSrc    string
	dataSize       string
	bootSize       string
	catalogFlag    bool
	publishBaseURL string
	usbGadget      bool
	kernelCfgPath  string
	withExternal   []string
	consoleBaud    int
	dataFlush      bool
	dataFilesystem string
	labelPrefix    string
	placeholders   []string
	ingressFlags   []string
	supportURL     string
	appVersion     string
)

// defaultDataSize is the data partition's size when --data-size is
// not given. It defaults to 0 (no data partition): persistence is opt-in, so
// appliance images that don't need /data don't pay its image-size and
// flash-time cost.
const defaultDataSize = "0"

// defaultBootSize is the boot partition's size when --boot-size is
// not given: today's locked constant, unchanged from before the flag
// existed. TestDefaultBootSizeMatchesImagePackage pins it against
// image.DefaultBootPartitionSizeBytes so the two can't silently drift apart.
const defaultBootSize = "256MiB"

// defaultDataFilesystem is the data partition's filesystem when
// --data-filesystem is not given: FAT32, unchanged from before the flag
// existed, because it's readable and repairable from any computer's SD card
// reader - the property an opt-in ext4 partition trades away for crash
// resilience (see docs/runtime.md).
const defaultDataFilesystem = "fat32"

func newBuildCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "build <path-to-main-package>",
		Short: "Cross-compile a Go app and assemble it into a bootable SD-card image",
		Long: `Cross-compile a Go app and assemble it into a bootable SD-card image.

Downloaded board artifacts (kernel/bootloader), the CA bundle, and any
--ingress binary are cached under your OS user cache dir (e.g.
~/Library/Caches/gosd on macOS, ~/.cache/gosd on Linux) so repeat builds
work fully offline. After a successful build, gosd automatically prunes any
cache entry left over from an older gosd version or pin, so the cache holds
only the current version's assets rather than growing forever - see
docs/artifacts.md. --artifacts-dir builds skip this pruning, since they may
not touch the cache at all.`,
		Args: cobra.ExactArgs(1),
		RunE: runBuild,
	}

	cmd.Flags().StringArrayVar(&boardIDs, "board", nil,
		fmt.Sprintf("board to build for (repeatable); omit to build all boards: %s", strings.Join(boards.IDs(), ", ")))
	cmd.Flags().StringVarP(&output, "output", "o", "",
		"output .img file when building one board, or output directory when building several")
	cmd.Flags().StringVar(&configDir, "config-dir", "",
		fmt.Sprintf("directory of setting files to overlay onto gosd's own %s/ tree: one value per file, each documented by a <name>%s sidecar (its own, or gosd's when the file only overrides a value); the merged tree is written to the boot partition, so a card's owner can edit it in any text editor (default: a %s directory beside the app's main package, when one exists)",
			configtree.Dir, configtree.DocSuffix, configtree.Dir))
	cmd.Flags().StringVar(&artifactsDir, "artifacts-dir", "",
		"directory of local kernel/firmware/bootloader files, checked before falling back to a pinned-URL download")
	cmd.Flags().StringVar(&gosdInitSrc, "gosd-init-src", os.Getenv("GOSD_INIT_SRC"),
		"directory containing gosd-init's main package source; overrides gosd's normal detection (dev checkout, then module cache) for unusual setups (default: $GOSD_INIT_SRC, the hook package managers use to point at their bundled copy); also locates cmd/gosd-tsfunnel's source (its gosd-tsfunnel sibling directory) when --ingress tailscale-funnel is selected")
	cmd.Flags().StringVar(&dataSize, "data-size", defaultDataSize,
		"size of the writable data partition (e.g. 512MiB, 2GiB), or 'expand' to keep the image small and have the device create the partition on first boot, filling the rest of the card; default 0 omits the partition entirely, so persistent /data is opt-in")
	cmd.Flags().StringVar(&bootSize, "boot-size", defaultBootSize,
		"size of the FAT32 boot partition (e.g. 512MiB, 2GiB); default 256MiB fits every stock board's kernel/initramfs, but a large app may need more - the build fails with an actionable error naming this flag if it doesn't fit; this size becomes part of the app's on-disk layout, so changing it in a later release erases the data partition on upgrade (see docs/design/upgrade-path.md §0.4)")
	cmd.Flags().BoolVar(&catalogFlag, "catalog", false,
		"also emit a Raspberry Pi Imager custom-repository os_list.json (per image, plus a combined file) alongside the built image(s); requires --publish-base-url")
	cmd.Flags().StringVar(&publishBaseURL, "publish-base-url", "",
		"absolute http(s) URL the built image(s) will be hosted at, used to build the catalog's download links; required by --catalog, and validated at build time like --support-url so a broken (or plaintext-http) link can't reach an end user's Imager")
	cmd.Flags().BoolVar(&usbGadget, "usb-gadget", false,
		"boot the board's USB port in peripheral mode, required if your app uses the gadget package (on the Pi Zero 2W this repurposes its only USB port from host to peripheral mode; no effect on Radxa Zero 3E)")
	cmd.Flags().StringVar(&kernelCfgPath, "kernel-config", "",
		fmt.Sprintf("developer kernel overlay config, read for its [[firmware]] entries only (default: %s in the working directory, if present)", defaultKernelConfigFile))
	cmd.Flags().StringArrayVar(&withExternal, "with-external", nil,
		"prebuilt static executable to bundle into the image at <path>[:<dest>] (repeatable); dest must be absolute, default /bin/<basename of path>; the binary must be a fully static ELF matching each selected board's architecture")
	cmd.Flags().IntVar(&consoleBaud, "console-baud", 0,
		"override the serial console baud rate baked into the boot config (e.g. 115200); default: each board's own rate (1500000 on the Rockchip boards, 115200 on the Pi boards) - useful when a USB-serial adapter can't reliably read the default rate (see COMPATIBILITY.md); the UART device itself (ttyS2, etc.) is unaffected, only its rate")
	cmd.Flags().BoolVar(&dataFlush, "data-flush", false,
		"mount the data partition, and any emmc/disk vfat volume, with the vfat \"flush\" option, pushing a file's data and metadata to the card promptly on close(2); default false uses normal Linux writeback (~30s dirty_expire) for faster writes, which is fine for apps using the documented durable-write pattern (fsync+rename, see docs/runtime.md#making-a-write-durable) - flush trades that write speed for prompter (but still not durable on its own) writeback; override per-device with the card's own config/data_flush setting")
	cmd.Flags().StringVar(&dataFilesystem, "data-filesystem", defaultDataFilesystem,
		"filesystem for the writable data partition, fat32 or ext4; default fat32 is readable in any computer's SD card reader, while ext4 is journaled and survives rapid power-off but cannot be read by macOS or Windows hosts; changing it between releases is an on-disk layout change like --boot-size, so an upgrading device's existing data partition is erased and re-established (see docs/design/upgrade-path.md)")
	cmd.Flags().StringVar(&labelPrefix, "label-prefix", "",
		fmt.Sprintf("prefix for the two partition volume labels, at most %d characters of [A-Za-z0-9_-]: the partitions are labelled <prefix>%s and <prefix>%s, so a flashed card shows up on a computer named after your app (default: the app's name, truncated to fit); the label is part of the app's on-disk layout like --boot-size, so changing it in a later release erases the data partition on upgrade (see docs/design/upgrade-path.md §0.4)",
			naming.LabelPrefixMaxLength, naming.BootLabelSuffix, naming.DataLabelSuffix))
	cmd.Flags().StringArrayVar(&placeholders, "placeholder", nil,
		"reserve a fixed-size comment-padded placeholder file on the boot partition at <path>=<size> (e.g. --placeholder backupist.yaml=32KiB, repeatable) and write a <image>.inject.json manifest beside each built image recording the absolute byte ranges a provisioning tool can overwrite with same-length bytes in the downloaded .img without any FAT tooling; see docs/image-injection.md")
	cmd.Flags().StringArrayVar(&ingressFlags, "ingress", nil,
		fmt.Sprintf("bake in a client that exposes an app's HTTP service to the public internet with zero app code (repeatable; supported values: %s); the tunnel itself is declared on-device in the card's config/ingress/<value>/ settings - cloudflared is arm64 boards only (its official arm release is GOARM=7 and faults on pi-zero-w's armv6), tailscale-funnel supports every board but needs a data partition (--data-size) to keep its tailnet identity across reboots", strings.Join(ingressAgentNames(), ", ")))
	cmd.Flags().StringVar(&supportURL, "support-url", "",
		"absolute http(s) URL for your app's support site, baked into config.json; the device points here in LAST_FATAL_ERROR.md when it has no specific fix to suggest (optional, but validated as an absolute http(s) URL at build time - a broken link in a crash report is worse than none)")
	cmd.Flags().StringVar(&appVersion, "app-version", "",
		"free-form version string for your app (e.g. 1.4.2), baked into config.json and shown in LAST_FATAL_ERROR.md's image line; never interpreted by gosd (optional - when omitted, the report falls back to the image's content-derived identity alone)")

	return cmd
}

func runBuild(cmd *cobra.Command, args []string) error {
	pkgPath := args[0]
	if err := validatePkgPath(pkgPath); err != nil {
		return err
	}

	if catalogFlag && publishBaseURL == "" {
		return fmt.Errorf("--catalog requires --publish-base-url=<https://...> so the generated os_list.json can build download links; try e.g. --publish-base-url=https://example.com/downloads")
	}
	resolvedPublishBaseURL, err := parsePublishBaseURL(publishBaseURL)
	if err != nil {
		return err
	}

	externalSpecs, err := parseWithExternalFlags(withExternal)
	if err != nil {
		return err
	}

	ingressSelected, err := parseIngressFlags(ingressFlags)
	if err != nil {
		return err
	}

	dataFS, err := parseDataFilesystem(dataFilesystem)
	if err != nil {
		return err
	}

	if err := validateDataFlushExt4Conflict(dataFS, dataFlush); err != nil {
		return err
	}

	dataSizeBytes, dataExpand, err := parseDataSize(dataSize, dataFS)
	if err != nil {
		return err
	}

	bootSizeBytes, err := parseBootSize(bootSize)
	if err != nil {
		return err
	}

	placeholderSpecs, err := parsePlaceholderFlags(placeholders)
	if err != nil {
		return err
	}

	resolvedSupportURL, err := parseSupportURL(supportURL)
	if err != nil {
		return err
	}

	if err := validateConsoleBaudRate(cmd, consoleBaud); err != nil {
		return err
	}

	selected, err := resolveBoards(boardIDs)
	if err != nil {
		return err
	}

	if err := validateUsbGadget(selected, usbGadget); err != nil {
		return err
	}

	if err := validateDataFilesystemSupport(selected, dataFS); err != nil {
		return err
	}

	if err := validateConsoleBaud(selected, consoleBaud); err != nil {
		return err
	}

	if err := validateIngress(selected, ingressSelected); err != nil {
		return err
	}

	if err := validateIngressDataPartition(ingressSelected, dataSizeBytes, dataExpand); err != nil {
		return err
	}

	// Last of the guards: everything above is about what the user typed, so
	// report a flag mistake before an environment one.
	if err := build.CheckToolchain(); err != nil {
		return err
	}

	appName, err := deriveAppName(pkgPath)
	if err != nil {
		return err
	}

	labels, err := resolveLabels(labelPrefix, cmd.Flags().Changed("label-prefix"), appName)
	if err != nil {
		return err
	}
	printPartitionLabels(cmd, "gosd build", labels, dataSizeBytes > 0 || dataExpand)

	resolvedConfigDir, err := resolveConfigDir(pkgPath, configDir)
	if err != nil {
		return err
	}

	outputs, err := resolveOutputs(selected, appName, output)
	if err != nil {
		return err
	}

	if err := ensureOutputDir(output, len(selected) > 1); err != nil {
		return err
	}

	tempDir, err := os.MkdirTemp("", "gosd-build-")
	if err != nil {
		return fmt.Errorf("creating a temp build directory failed: %w", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	binaries, err := compileForBoards(selected, tempDir, pkgPath, gosdInitSrc, ingressSelected.TailscaleFunnel, build.CrossCompile, build.CrossCompileGosdInit, build.CrossCompileTsfunnel)
	if err != nil {
		return err
	}

	ctx := cmd.Context()
	cacheDir, err := artifactCacheDir()
	if err != nil {
		return err
	}

	firmwarePaths, err := loadKernelConfigFirmware(ctx, kernelCfgPath)
	if err != nil {
		return err
	}

	var ingressGOARCHesNeeded []string
	if ingressSelected.Cloudflared {
		ingressGOARCHesNeeded = ingressGOARCHes(selected)
	}
	shared, err := resolveSharedContent(ctx, artifactsDir, ingressGOARCHesNeeded)
	if err != nil {
		return err
	}

	for _, b := range selected {
		bin := binaries[b.Name()]

		extraFirmware, err := openKernelFirmware(firmwarePaths)
		if err != nil {
			return err
		}

		extraExecutables, err := openExternalsForBoard(externalSpecs, b)
		if err != nil {
			return err
		}

		extraFiles, ingressExecutables, err := openSharedContent(shared, b)
		if err != nil {
			return err
		}
		for dest, r := range ingressExecutables {
			if extraExecutables == nil {
				extraExecutables = make(map[string]io.Reader, len(ingressExecutables))
			}
			extraExecutables[dest] = r
		}

		if bin.tsfunnelPath != "" {
			tf, err := openTsfunnelBinary(bin.tsfunnelPath)
			if err != nil {
				return err
			}
			if extraExecutables == nil {
				extraExecutables = make(map[string]io.Reader, 1)
			}
			extraExecutables[ingressTailscaleFunnelDest] = tf
		}

		tree, err := configtree.Build(resolvedConfigDir, ingressFeaturesFor(ingressSelected, b))
		if err != nil {
			return fmt.Errorf("assembling the %s configuration for %s failed: %w", configtree.Dir, b.Name(), err)
		}

		opts := pipeline.Options{
			Board:          b,
			AppBinaryPath:  bin.appPath,
			InitBinaryPath: bin.initPath,
			Config: boards.BuildConfig{
				Hostname:    appName,
				UsbGadget:   usbGadget,
				ConsoleBaud: consoleBaud,
			},
			ConfigTree:             tree,
			ArtifactsDir:           artifactsDir,
			CacheDir:               cacheDir,
			OutputPath:             outputs[b.Name()],
			Labels:                 labels,
			DataSizeBytes:          dataSizeBytes,
			DataExpand:             dataExpand,
			DataFlush:              dataFlush,
			DataFilesystem:         dataFS,
			BootSizeBytes:          bootSizeBytes,
			ExtraFirmware:          extraFirmware,
			ExtraExecutables:       extraExecutables,
			ExtraFiles:             extraFiles,
			Placeholders:           placeholderSpecs,
			IngressCloudflared:     ingressSelected.Cloudflared,
			IngressTailscaleFunnel: ingressSelected.TailscaleFunnel,
			AppName:                appName,
			AppVersion:             appVersion,
			SupportURL:             resolvedSupportURL,
		}
		report, err := pipeline.Assemble(ctx, opts)
		if err != nil {
			if errors.Is(err, image.ErrBootPartitionFull) {
				return fmt.Errorf("building %s for %s failed: %w; pass a larger --boot-size than %s and rebuild", appName, b.Name(), err, humanizeBinaryBytes(bootSizeBytes))
			}
			return fmt.Errorf("building %s for %s failed: %w", appName, b.Name(), err)
		}
		printBootVolumeUsage(cmd, b.Name(), report)

		manifestPath, err := inject.WriteManifest(outputs[b.Name()], inject.ManifestSpec{
			Board:        b.Name(),
			Placeholders: placeholderSpecs,
			Config:       tree,
			FileRanges:   report.FileRanges,
		})
		if err != nil {
			return fmt.Errorf("writing the injection manifest for %s (%s) failed: %w", appName, b.Name(), err)
		}
		cmd.PrintErrf("gosd build: %s inject manifest: %s\n", b.Name(), manifestPath)
	}

	pruneDownloadCaches(cmd, artifactsDir)

	if catalogFlag {
		if err := writeCatalog(cmd, selected, appName, outputs, resolvedPublishBaseURL); err != nil {
			return err
		}
	}

	return nil
}

// validatePkgPath checks that the positional operand `gosd build` and `gosd
// run` were handed really is a path to a Go package, before anything hands
// it to the Go toolchain.
//
// It exists because cobra honours "--" as a flag terminator, so `gosd build
// -- -toolexec=/tmp/payload` puts an arbitrary Go build flag in args[0];
// unterminated, `go build` reads it as a flag, and -toolexec runs a program
// of the attacker's choosing on the build host before the app is even
// compiled (bean gosd-jc24). Any wrapper forwarding a value it doesn't fully
// control - a CI job templating a branch-derived path, a Makefile's
// `gosd build $(PKG)` - is how such a value arrives.
//
// The check is an allow-list on the shape of a package path, never a
// blacklist of flag names: -toolexec is only today's worst flag, and
// -ldflags, -exec, -overlay and whatever the toolchain gains next would each
// have to be remembered. Anything not recognisably a package path is
// refused.
func validatePkgPath(pkgPath string) error {
	if packagePathLike(pkgPath) {
		return nil
	}
	return fmt.Errorf(
		"%q is not a path to a Go package, so gosd won't hand it to the Go toolchain (an argument starting with \"-\" is read as a build flag rather than a package, and flags such as -toolexec run arbitrary programs on this machine); "+
			"pass the directory holding your app's main package instead: \".\" for the working directory, \"./cmd/myapp\", or an absolute path",
		pkgPath)
}

// packagePathLike reports whether s has the shape of a Go package operand: a
// relative path (".", "..", "./x", "../x"), an absolute path, or an import
// path ("github.com/you/app"). The Go toolchain forbids a leading dash in
// every import path element ("malformed import path: leading dash"), so
// requiring a package path to begin the way one legitimately can is that
// same rule stated positively.
func packagePathLike(s string) bool {
	if s == "." || s == ".." || strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../") {
		return true
	}
	if filepath.IsAbs(s) {
		return true
	}
	r, size := utf8.DecodeRuneInString(s)
	if size == 0 || r == utf8.RuneError {
		return false
	}
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

// deriveAppName computes the default app name gosd uses for the device
// hostname (config.json's baked fallback, used whenever the card's
// config/hostname is left unset) and output filenames: the sanitized basename of pkgPath's
// directory. pkgPath is resolved to an absolute path first so that "." (the
// README quickstart's canonical `gosd build .` invocation) yields the
// working directory's name rather than filepath.Base(".") == ".", which
// naming.Sanitize reduces to "" and falls back to "app".
func deriveAppName(pkgPath string) (string, error) {
	abs, err := filepath.Abs(pkgPath)
	if err != nil {
		return "", fmt.Errorf("resolving %q to an absolute path failed: %w; check the path exists and is accessible", pkgPath, err)
	}
	return naming.Sanitize(filepath.Base(abs)), nil
}

// defaultConfigDirName is the directory `gosd build` picks up as the app's
// config overlay with no --config-dir: a config/ directory beside the app's
// main package, which is where an app's settings naturally live in its own
// repository.
const defaultConfigDirName = configtree.Dir

// resolveConfigDir decides which directory (if any) overlays gosd's own
// config defaults. An explicit --config-dir must exist - a typo'd path
// silently building gosd's bare defaults would ship an image missing every
// setting the app actually needs - while the default beside the main package
// is only used when it happens to be there.
func resolveConfigDir(pkgPath, flag string) (string, error) {
	if flag != "" {
		info, err := os.Stat(flag)
		if err != nil {
			return "", fmt.Errorf("--config-dir %s can't be read: %w; point it at a directory holding one file per setting, or drop the flag to build with gosd's own defaults", flag, err)
		}
		if !info.IsDir() {
			return "", fmt.Errorf("--config-dir %s is a file, not a directory; it takes a directory holding one file per setting", flag)
		}
		return flag, nil
	}

	abs, err := filepath.Abs(pkgPath)
	if err != nil {
		return "", fmt.Errorf("resolving %q to an absolute path failed: %w; check the path exists and is accessible", pkgPath, err)
	}
	candidate := filepath.Join(abs, defaultConfigDirName)
	if info, err := os.Stat(candidate); err == nil && info.IsDir() {
		return candidate, nil
	}
	return "", nil
}

// binarySizeUnits are the size suffixes --data-size and --boot-size both
// accept, all binary (power-of-1024) units: partition sizes are
// conventionally binary, and offering only one interpretation avoids
// MB-vs-MiB ambiguity.
var binarySizeUnits = map[string]int64{
	"KIB": 1024,
	"MIB": 1024 * 1024,
	"GIB": 1024 * 1024 * 1024,
	"K":   1024,
	"M":   1024 * 1024,
	"G":   1024 * 1024 * 1024,
}

// parseSizeBytes parses the numeric core --data-size and --boot-size both
// share - a bare number of bytes, or one with a binarySizeUnits suffix (e.g.
// "512MiB", "2G") - into bytes. flagName is used only to name the flag in the
// returned error. Callers layer their own keywords (--data-size's "expand")
// and bounds (max/min/alignment) on top.
func parseSizeBytes(flagName, s string) (int64, error) {
	trimmed := strings.TrimSpace(s)

	numPart := trimmed
	var multiplier int64 = 1
	for suffix, mult := range binarySizeUnits {
		if n, ok := strings.CutSuffix(strings.ToUpper(trimmed), suffix); ok {
			numPart, multiplier = strings.TrimSpace(n), mult
			break
		}
	}

	n, err := strconv.ParseInt(numPart, 10, 64)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("%s %q is not a valid size; use a number with a binary unit (e.g. 512MiB, 1GiB)", flagName, s)
	}
	if multiplier > 1 && n > math.MaxInt64/multiplier {
		return 0, fmt.Errorf("%s %q is too large; choose something that fits on an SD card", flagName, s)
	}
	return n * multiplier, nil
}

// dataSizeLimitDocsURL is where the --data-size refusal sends a developer who
// asked for a partition GoSD's FAT32 formatter cannot lay out: the runtime
// contract's account of the ceiling and what to reach for instead.
const dataSizeLimitDocsURL = "https://github.com/jphastings/gosd/blob/main/docs/runtime.md#how-big-the-data-partition-can-be"

// parseDataSize parses a --data-size value like "512MiB", "2G", or "0" into
// bytes, or the keyword "expand" (ship no data partition in the image and
// have gosd-init create one filling the rest of the card on first boot). A
// bare number is bytes; 0 (with or without a unit) disables the data
// partition, except for ext4 (see below), which needs a partition to exist
// at all.
//
// fs decides which bound applies, since FAT32 and ext4 fail in opposite
// directions: a FAT32 partition past diskfmt.MaxFAT32Bytes() is refused here
// before any image bytes exist, rather than after a long build produces a
// silently corrupt partition (see diskfmt.FAT32SizeLimitReason) - that
// ceiling is a defect of GoSD's own FAT32 formatter, so it does not apply to
// ext4 at all. ext4 instead has a floor: GoSD writes a fixed
// diskfmt.MinEXT4Bytes() golden image and grows it to the partition's real
// size on first boot, so a smaller partition has nowhere to grow into (see
// diskfmt.EXT4SizeLimitReason), and 0 (no partition) is refused outright,
// since --data-filesystem=ext4 with no partition to format is certainly a
// mistake. "expand" is valid for either filesystem and is resolved before
// either bound is checked, since it carries no --data-size number to compare
// against a ceiling or floor - a --data-size=expand ext4 image genuinely
// fills the whole card, floor included. Both filesystems also share a
// sub-sector floor (image.SectorSizeBytes): a size that rounds down to zero
// whole sectors is refused here too, up front, rather than deep inside
// image.computeLayout after a full build.
func parseDataSize(s string, fs diskfmt.FS) (bytes int64, expand bool, err error) {
	trimmed := strings.TrimSpace(s)
	if strings.EqualFold(trimmed, "expand") {
		return 0, true, nil
	}

	size, err := parseSizeBytes("--data-size", s)
	if err != nil {
		return 0, false, fmt.Errorf("%w, 'expand' to fill the card on first boot, or 0 to disable the data partition", err)
	}

	// A size that rounds down to zero whole sectors can never back a
	// partition - image.computeLayout refuses it too, but only after a
	// full cross-compile and artifact fetch for every board. Catching it
	// here keeps that refusal instant, matching the ceiling check below
	// and the ext4 floor's own "fail before any image bytes exist"
	// contract.
	if size > 0 && size < image.SectorSizeBytes {
		return 0, false, fmt.Errorf("--data-size %q (%d bytes) is smaller than one sector (%d bytes), which GoSD's image writer can never format as a partition; double-check you didn't forget a unit suffix (e.g. --data-size=100MiB, not --data-size=100), or pass --data-size=0 to disable the data partition entirely",
			s, size, image.SectorSizeBytes)
	}

	if fs == diskfmt.EXT4 {
		if size == 0 {
			return 0, false, fmt.Errorf("--data-filesystem=ext4 needs a writable data partition to format, but --data-size=0 (the default) means none is created; pass --data-size (e.g. --data-size=1GiB) or --data-size=expand, or drop --data-filesystem=ext4 to build without a data partition")
		}
		if minBytes := diskfmt.MinEXT4Bytes(); size < minBytes {
			return 0, false, fmt.Errorf("--data-size %q is smaller than the %s minimum GoSD's ext4 formatter needs, because %s; use --data-size=%s or larger (--data-size=%d for the exact minimum), or --data-size=expand which always clears it, or build with --data-filesystem=fat32 instead",
				s, humanizeBinaryBytes(minBytes), diskfmt.EXT4SizeLimitReason, humanizeBinaryBytes(minBytes), minBytes)
		}
		return size, false, nil
	}

	if size > diskfmt.MaxFAT32Bytes() {
		return 0, false, fmt.Errorf("--data-size %q is larger than GoSD can format: the largest data partition it will create is %s (%d bytes), because %s; use --data-size=256GiB or less (--data-size=%d for the exact maximum), or --data-size=expand to fill the card up to 256GiB on first boot; if the app needs more storage than that, attach a disk and format it exFAT with the disk package, which has no such ceiling - see %s",
			s, diskfmt.GibibytesString(diskfmt.MaxFAT32Bytes()), diskfmt.MaxFAT32Bytes(), diskfmt.FAT32SizeLimitReason, diskfmt.MaxFAT32Bytes(), dataSizeLimitDocsURL)
	}
	return size, false, nil
}

// dataFilesystemNames lists every valid --data-filesystem value, in the
// order shown in an unknown-value error - the same style
// ingressAgentNames() uses for --ingress.
var dataFilesystemNames = []string{"fat32", "ext4"}

// parseDataFilesystem validates --data-filesystem's raw value against
// dataFilesystemNames, case-insensitively, and resolves it to the
// diskfmt.FS token the rest of the build threads through
// (pipeline.Options.DataFilesystem, image.Spec.DataFilesystem,
// initcfg.Config.DataFilesystem) - mirroring parseIngressFlags' unknown-value
// error shape.
func parseDataFilesystem(s string) (diskfmt.FS, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "fat32":
		return diskfmt.FAT32, nil
	case "ext4":
		return diskfmt.EXT4, nil
	default:
		return "", fmt.Errorf("--data-filesystem %q is invalid; valid values are: %s", s, strings.Join(dataFilesystemNames, ", "))
	}
}

// validateDataFlushExt4Conflict refuses combining --data-filesystem=ext4
// with --data-flush: "flush" is a vfat "flush" mount option
// (initcfg.Config.DataFlush), meaningless for ext4, whose durability comes
// from the fsync/rename sequence in docs/runtime.md either way - the same
// story the flag's own --data-flush help text tells for FAT32. A no-op
// unless both are set.
func validateDataFlushExt4Conflict(fs diskfmt.FS, dataFlush bool) error {
	if fs != diskfmt.EXT4 || !dataFlush {
		return nil
	}
	return fmt.Errorf("--data-filesystem=ext4 can't be combined with --data-flush: \"flush\" is a vfat-only mount option and has no effect on an ext4 data partition - durability already comes from the fsync sequence documented in docs/runtime.md regardless of filesystem; drop --data-flush, or drop --data-filesystem=ext4 to use FAT32 (which --data-flush does affect)")
}

// validateDataFilesystemSupport fails fast when --data-filesystem=ext4 is
// selected and any board in selected has no ext4-capable stock kernel (see
// boards.Board.EXT4Support) - without this check, gosd build
// --data-filesystem=ext4 for such a board would either fail deep inside
// image.Write or, worse, ship an image whose data partition the kernel can never
// mount. Mirrors validateUsbGadget's shape exactly, including naming
// --board as the fix: remember a bare `gosd build` with no --board builds
// every public board, so an ext4 refusal must point at restricting the
// build, not just "this doesn't work". A no-op when fs is FAT32 or every
// selected board supports ext4.
func validateDataFilesystemSupport(selected []boards.Board, fs diskfmt.FS) error {
	if fs != diskfmt.EXT4 {
		return nil
	}

	var incapable, capable []string
	for _, b := range selected {
		support := b.EXT4Support()
		if support.Supported {
			capable = append(capable, b.Name())
			continue
		}
		incapable = append(incapable, fmt.Sprintf("%s (%s)", b.Name(), support.Reason))
	}
	if len(incapable) == 0 {
		return nil
	}

	msg := fmt.Sprintf(
		"--data-filesystem=ext4 failed: no ext4 support in the pinned kernel for %s; see COMPATIBILITY.md's ext4 data partition row",
		strings.Join(incapable, "; "),
	)
	if len(capable) > 0 {
		msg += fmt.Sprintf("; other selected boards do support ext4 (%s) — try restricting the build with --board=%s, or drop --data-filesystem=ext4 to use FAT32 (the default) across every board",
			strings.Join(capable, ", "), capable[0])
	} else {
		msg += "; drop --data-filesystem=ext4 to use FAT32 (the default) instead"
	}
	return errors.New(msg)
}

// minBootSizeBytes is the smallest --boot-size GoSD will accept: not because
// any real board's kernel+initramfs could fit in 1MiB (they can't - the
// actual fit is checked at build time, once the payload is known, and
// refused with ErrBootPartitionFull if it doesn't fit), but because anything
// smaller is certainly a mistake, most likely a missing unit suffix (e.g.
// --boot-size=256 meaning 256 bytes instead of 256MiB).
const minBootSizeBytes = 1024 * 1024

// bootSizeAlignmentBytes is the granularity --boot-size values must land on:
// a whole number of MiB, so the flag can't accidentally desynchronize the
// data partition's start from a round boundary a card's own erase-block
// alignment would want anyway.
const bootSizeAlignmentBytes = 1024 * 1024

// parseBootSize parses a --boot-size value like "512MiB" or "2G" into bytes.
// Unlike --data-size there is no "0 disables it" or "expand" case - every
// image has exactly one boot partition - so an empty/default flag value
// resolves to image.DefaultBootPartitionSizeBytes via Spec.BootSizeBytes's
// own zero-means-default handling, and parseBootSize itself only ever
// returns a concrete positive size or an error naming --boot-size.
func parseBootSize(s string) (int64, error) {
	size, err := parseSizeBytes("--boot-size", s)
	if err != nil {
		return 0, err
	}
	if size < minBootSizeBytes {
		return 0, fmt.Errorf("--boot-size %q (%d bytes) is smaller than the %d-byte minimum GoSD will format as a boot partition; double-check you didn't forget a unit suffix (e.g. --boot-size=256MiB, not --boot-size=256) - whether your app's own kernel and initramfs actually fit is checked at build time, not here",
			s, size, minBootSizeBytes)
	}
	if size > diskfmt.MaxFAT32Bytes() {
		return 0, fmt.Errorf("--boot-size %q is larger than GoSD can format: the largest boot partition it will create is %s (%d bytes), because %s",
			s, diskfmt.GibibytesString(diskfmt.MaxFAT32Bytes()), diskfmt.MaxFAT32Bytes(), diskfmt.FAT32SizeLimitReason)
	}
	if size%bootSizeAlignmentBytes != 0 {
		return 0, fmt.Errorf("--boot-size %q (%d bytes) is not a whole number of MiB; round it to the nearest MiB, e.g. --boot-size=%dMiB",
			s, size, (size+bootSizeAlignmentBytes/2)/bootSizeAlignmentBytes)
	}
	return size, nil
}

// parsePlaceholderFlags turns the repeated --placeholder <path>=<size> flag
// values into inject.Placeholder specs. Each is split on the FIRST '=' (a
// size can't contain one), sized via parseSizeBytes (the same binary-unit
// parser --data-size/--boot-size share), and validated (path shape,
// minimum/maximum size - see inject.Placeholder.Validate) before any
// compilation starts, so a bad --placeholder fails fast. A path given more
// than once, case-insensitively (FAT paths are case-insensitive), is
// rejected outright: a duplicate is far more likely a mistake than an
// intentional override, and the intentional case still has a clear fix.
func parsePlaceholderFlags(flags []string) ([]inject.Placeholder, error) {
	if len(flags) == 0 {
		return nil, nil
	}

	placeholders := make([]inject.Placeholder, 0, len(flags))
	seen := make(map[string]string, len(flags))
	for _, flag := range flags {
		pathPart, sizePart, ok := strings.Cut(flag, "=")
		if !ok {
			return nil, fmt.Errorf("--placeholder %q is invalid; use --placeholder <path>=<size> (e.g. --placeholder backupist.yaml=32KiB)", flag)
		}

		size, err := parseSizeBytes("--placeholder", sizePart)
		if err != nil {
			return nil, err
		}

		p := inject.Placeholder{Path: pathPart, SizeBytes: size}
		if err := p.Validate(); err != nil {
			return nil, err
		}

		if existing, dup := seen[strings.ToLower(p.Path)]; dup {
			return nil, fmt.Errorf("--placeholder path %q was given more than once (as %q); FAT paths are case-insensitive, so pick a distinct path for each --placeholder", p.Path, existing)
		}
		seen[strings.ToLower(p.Path)] = p.Path

		placeholders = append(placeholders, p)
	}
	return placeholders, nil
}

// parseSupportURL validates --support-url: when given, it must be an
// absolute http(s) URL (a scheme and a host), because a broken link in a
// crash report is worse than no link at all - a device's owner who follows
// it has no recourse. Empty is valid: the flag is optional, and omitting it
// just means LAST_FATAL_ERROR.md's fallback fix text has no link to offer
// (see bean gosd-my8e).
func parseSupportURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}

	u, err := url.Parse(trimmed)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("--support-url %q is invalid; it must be an absolute http:// or https:// URL with a host, e.g. https://example.com/support", raw)
	}
	return trimmed, nil
}

// parsePublishBaseURL validates --publish-base-url the same way
// parseSupportURL validates --support-url: it must be an absolute http(s)
// URL with a host, because every download link in the generated
// os_list.json is built from it and lands in an end user's Raspberry Pi
// Imager. Empty is valid — the flag is only required by --catalog, which
// checks for it separately.
func parsePublishBaseURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}
	u, err := url.Parse(trimmed)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("--publish-base-url %q is invalid; it must be an absolute http:// or https:// URL with a host, e.g. --publish-base-url=https://example.com/downloads", raw)
	}
	return trimmed, nil
}

// humanizeBinaryBytes renders a byte count the way a developer thinks about
// partition sizes: whole-number MiB below 1GiB, two-decimal GiB above.
func humanizeBinaryBytes(n int64) string {
	const gib = 1024 * 1024 * 1024
	if n >= gib {
		return fmt.Sprintf("%.2fGiB", float64(n)/gib)
	}
	return fmt.Sprintf("%dMiB", n/(1024*1024))
}

// printBootVolumeUsage prints a one-line boot-volume usage summary
// (`gosd build`'s per-board headroom report, bean gosd-m70t) so a developer
// watches their app's footprint against --boot-size shrink across releases,
// long before it trips ErrBootPartitionFull.
func printBootVolumeUsage(cmd *cobra.Command, boardName string, report image.WriteReport) {
	var percent float64
	if report.BootPartitionSizeBytes > 0 {
		percent = 100 * float64(report.BootPartitionPayloadBytes) / float64(report.BootPartitionSizeBytes)
	}
	cmd.PrintErrf("gosd build: %s boot volume: %s / %s used (%.1f%%)\n",
		boardName, humanizeBinaryBytes(report.BootPartitionPayloadBytes), humanizeBinaryBytes(report.BootPartitionSizeBytes), percent)
}

// writeCatalog builds and writes the Raspberry Pi Imager custom-repository
// catalog (--catalog) for the images just built at outputs, reading each
// finished .img back off disk to compute its size/hash. All of selected's
// images share one output directory (resolveOutputs always maps every
// board into the same directory when there's more than one, and a single
// board's own directory when there's just one), so the combined
// os_list.json is written next to the first image.
//
// Internal-only boards (currently just qemu-virt - see this file's init())
// are never listed in a catalog end users are meant to paste
// into Imager, so they're filtered out of selected before any entry is
// built - not because they'd fail, but because a catalog is a genuinely
// public artifact. A build of only internal boards (e.g. `--board=qemu-virt
// --catalog`) is therefore a silent no-op: nothing to write isn't an error,
// and --catalog on a normal, public-board build is unaffected either way.
func writeCatalog(cmd *cobra.Command, selected []boards.Board, appName string, outputs map[string]string, baseURL string) error {
	images := make([]catalog.Image, 0, len(selected))
	for _, b := range selected {
		if boards.IsInternal(b.Name()) {
			continue
		}
		images = append(images, catalog.Image{
			AppName: appName,
			BoardID: b.Name(),
			Path:    outputs[b.Name()],
		})
	}
	if len(images) == 0 {
		cmd.PrintErrln("gosd build --catalog: every selected board is internal-only, so no catalog entries were written")
		return nil
	}

	dir := filepath.Dir(images[0].Path)
	if _, err := catalog.WriteFiles(dir, images, catalog.Options{BaseURL: baseURL}); err != nil {
		return fmt.Errorf("writing the Imager catalog failed: %w", err)
	}
	return nil
}

// artifactCacheDir returns the directory pinned-URL artifact downloads are
// cached in across builds, so a board's firmware isn't re-fetched every
// run.
func artifactCacheDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("locating a user cache directory for downloaded artifacts failed: %w; try passing --artifacts-dir instead", err)
	}
	return filepath.Join(base, "gosd", "artifacts"), nil
}

// resolveBoards turns the --board flag values into a de-duplicated list of
// registered boards, defaulting to every registered board when none are
// given.
func resolveBoards(ids []string) ([]boards.Board, error) {
	if len(ids) == 0 {
		return boards.All(), nil
	}

	seen := make(map[string]bool, len(ids))
	selected := make([]boards.Board, 0, len(ids))
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true

		b, ok := boards.Find(id)
		if !ok {
			return nil, fmt.Errorf("unknown board %q; try one of: %s", id, strings.Join(boards.IDs(), ", "))
		}
		selected = append(selected, b)
	}
	return selected, nil
}

// validateUsbGadget fails fast when --usb-gadget is set and any board in
// selected has no USB peripheral controller at its pinned artifacts (see
// boards.Board.UsbGadgetSupport) - without this check, `gosd build
// --usb-gadget` for such a board succeeds and produces an image whose app
// can never find a UDC at /sys/class/udc, with no earlier warning than
// gosd-init's own boot-time log line (see COMPATIBILITY.md's USB gadget
// row). A no-op when usbGadget is false or every selected board supports it.
func validateUsbGadget(selected []boards.Board, usbGadget bool) error {
	if !usbGadget {
		return nil
	}

	var incapable, capable []string
	for _, b := range selected {
		support := b.UsbGadgetSupport()
		if support.Supported {
			capable = append(capable, b.Name())
			continue
		}
		incapable = append(incapable, fmt.Sprintf("%s (%s)", b.Name(), support.Reason))
	}
	if len(incapable) == 0 {
		return nil
	}

	msg := fmt.Sprintf(
		"--usb-gadget failed: no USB peripheral controller at the pinned artifacts for %s; see COMPATIBILITY.md's USB gadget row",
		strings.Join(incapable, "; "),
	)
	if len(capable) > 0 {
		msg += fmt.Sprintf("; other selected boards do support --usb-gadget (%s) — try restricting the build with --board=%s",
			strings.Join(capable, ", "), capable[0])
	}
	return errors.New(msg)
}

// consoleBaudCommonRates are typical UART baud rates. --console-baud accepts
// any positive integer - a board's kernel/bootloader driver dictates what
// actually works, and this flag's whole purpose is working around USB-serial
// adapters (CP210x, PL2303, ...) GoSD can't enumerate in advance, so nothing
// here is treated as a hard limit - but a value outside this set is far more
// likely to be a typo (a dropped digit, say) than an intentional exotic
// rate, so validateConsoleBaudRate warns rather than silently accepting it.
var consoleBaudCommonRates = map[int]bool{
	9600: true, 19200: true, 38400: true, 57600: true, 115200: true,
	230400: true, 460800: true, 921600: true, 1500000: true, 3000000: true,
}

// validateConsoleBaudRate checks --console-baud's raw value before any board
// resolution happens. 0 is the flag's default (meaning "not passed") and
// always succeeds, leaving every board's own rate unchanged. Any positive
// integer is accepted; one outside consoleBaudCommonRates prints a warning
// to cmd's stderr but still proceeds - permissive-with-warning, since a
// board/adapter pair genuinely needing an unusual rate is a real (if rare)
// case this flag exists to serve, not something to block outright.
func validateConsoleBaudRate(cmd *cobra.Command, rate int) error {
	if rate < 0 {
		return fmt.Errorf("--console-baud %d is invalid; give a positive number of bits per second (e.g. 115200), or omit the flag to keep each board's own default", rate)
	}
	if rate > 0 && !consoleBaudCommonRates[rate] {
		cmd.PrintErrf("gosd build --console-baud %d: not a commonly used baud rate; continuing, but double-check your adapter and board both actually support it\n", rate)
	}
	return nil
}

// validateConsoleBaud fails fast when --console-baud is set (non-zero) and
// any board in selected can't honor a boot-config baud override (see
// boards.Board.ConsoleBaudSupport) - same shape as validateUsbGadget's
// capability check, so a board whose boot config has no console= at all
// (currently just qemu-virt) fails loudly instead of silently ignoring the
// flag. A no-op when consoleBaud is 0 (flag not passed) or every selected
// board supports it.
func validateConsoleBaud(selected []boards.Board, consoleBaud int) error {
	if consoleBaud == 0 {
		return nil
	}

	var incapable, capable []string
	for _, b := range selected {
		support := b.ConsoleBaudSupport()
		if support.Supported {
			capable = append(capable, b.Name())
			continue
		}
		incapable = append(incapable, fmt.Sprintf("%s (%s)", b.Name(), support.Reason))
	}
	if len(incapable) == 0 {
		return nil
	}

	msg := fmt.Sprintf(
		"--console-baud failed: %s cannot honor a console baud override",
		strings.Join(incapable, "; "),
	)
	if len(capable) > 0 {
		msg += fmt.Sprintf("; other selected boards do support --console-baud (%s) — try restricting the build with --board=%s",
			strings.Join(capable, ", "), capable[0])
	}
	return errors.New(msg)
}

// ensureOutputDir makes sure the directory gosd is about to write into
// already exists, creating it (and any missing parents) if not. In
// multi-board mode output itself names that directory; in single-board mode
// output names the .img file, so only its parent directory needs to exist.
// An empty output (single-board mode with no --output) writes into the
// current directory, which always exists, so there's nothing to do.
func ensureOutputDir(output string, multiBoard bool) error {
	dir := output
	if multiBoard {
		if dir == "" {
			dir = "."
		}
	} else if dir == "" {
		return nil
	} else {
		dir = filepath.Dir(dir)
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		if multiBoard {
			if info, statErr := os.Stat(dir); statErr == nil && !info.IsDir() {
				return fmt.Errorf("-o must be a directory when building multiple boards; %s is a file", dir)
			}
		}
		return fmt.Errorf("creating output directory %s failed: %w; check the path is writable and try a different -o", dir, err)
	}
	return nil
}

// resolveOutputs maps each selected board to its output .img path. When
// exactly one board is selected, --output (if given) names that file
// directly. Otherwise --output (if given) names the directory the
// per-board <appname>-<board>.img files are written into.
func resolveOutputs(selected []boards.Board, appName, output string) (map[string]string, error) {
	outputs := make(map[string]string, len(selected))

	if len(selected) == 1 {
		b := selected[0]
		path := output
		if path == "" {
			path = fmt.Sprintf("%s-%s.img", appName, b.Name())
		}
		outputs[b.Name()] = path
		return outputs, nil
	}

	dir := output
	if dir == "" {
		dir = "."
	}
	for _, b := range selected {
		outputs[b.Name()] = filepath.Join(dir, fmt.Sprintf("%s-%s.img", appName, b.Name()))
	}
	return outputs, nil
}
