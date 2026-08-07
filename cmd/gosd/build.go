package main

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jphastings/gosd/internal/boards"
	"github.com/jphastings/gosd/internal/boards/cubiea5e"
	"github.com/jphastings/gosd/internal/boards/nanopizero2"
	"github.com/jphastings/gosd/internal/boards/pi3b"
	"github.com/jphastings/gosd/internal/boards/pizero2w"
	"github.com/jphastings/gosd/internal/boards/pizerow"
	"github.com/jphastings/gosd/internal/boards/qemuvirt"
	"github.com/jphastings/gosd/internal/boards/radxazero3e"
	"github.com/jphastings/gosd/internal/boards/rock4se"
	"github.com/jphastings/gosd/internal/build"
	"github.com/jphastings/gosd/internal/catalog"
	"github.com/jphastings/gosd/internal/diskfmt"
	"github.com/jphastings/gosd/internal/image"
	"github.com/jphastings/gosd/internal/inject"
	"github.com/jphastings/gosd/internal/naming"
	"github.com/jphastings/gosd/internal/pipeline"
)

func init() {
	boards.Register(pizero2w.New())
	boards.Register(pizerow.New())
	boards.Register(radxazero3e.New())
	// nanopi-zero2 is public: gosd-f39b's U-Boot artifact pipeline entries
	// are published in the artifacts/v0.2.0 release, so real
	// (non---artifacts-dir) fetches for this board now resolve.
	boards.Register(nanopizero2.New())
	// qemu-virt is internal-only (see CLAUDE.md's locked decision): it's a
	// real, fully buildable board, but only reachable via an explicit
	// --board=qemu-virt, never part of the default no---board build set,
	// --help text, or catalog generation.
	boards.RegisterInternal(qemuvirt.New())
	// rock-4se is public: its kernel and U-Boot (TF-A compiled from
	// source, no rkbin blobs) are published in the artifacts/v0.5.0
	// release, so real (non---artifacts-dir) fetches for this board now
	// resolve (bean gosd-h8a8's activation flip).
	boards.Register(rock4se.New())
	// pi-3b is public: its kernel and both family DTBs (one image covers
	// the 3B and the 3B+) are published in the artifacts/v0.8.0 release,
	// so real (non---artifacts-dir) fetches for this board now resolve
	// (bean gosd-7wv9's activation flip).
	boards.Register(pi3b.New())
	// cubie-a5e is internal-only (epic gosd-h1wv): the board profile is
	// registered so gosd build-kernel and the U-Boot pipeline beans can
	// resolve it (the same de-facto prerequisite rock-4se's registration
	// was for its kernel build), but no kernel or U-Boot artifacts exist
	// yet - only reachable via an explicit --board=cubie-a5e until a
	// later activation bean publishes its artifacts and flips it public.
	boards.RegisterInternal(cubiea5e.New())
}

var (
	boardIDs       []string
	output         string
	hostname       string
	wifiSSID       string
	wifiPass       string
	artifactsDir   string
	gosdInitSrc    string
	dataSize       string
	bootSize       string
	catalogFlag    bool
	publishBaseURL string
	usbGadget      bool
	envFlags       []string
	kernelCfgPath  string
	withExternal   []string
	consoleBaud    int
	dataFlush      bool
	placeholders   []string
)

// defaultDataSize is the GOSD-DATA partition size used when --data-size is
// not given. It defaults to 0 (no data partition): persistence is opt-in, so
// appliance images that don't need /data don't pay its image-size and
// flash-time cost.
const defaultDataSize = "0"

// defaultBootSize is the GOSD-BOOT partition size used when --boot-size is
// not given: today's locked constant, unchanged from before the flag
// existed. TestDefaultBootSizeMatchesImagePackage pins it against
// image.DefaultBootPartitionSizeBytes so the two can't silently drift apart.
const defaultBootSize = "256MiB"

func newBuildCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "build <path-to-main-package>",
		Short: "Cross-compile a Go app and assemble it into a bootable SD-card image",
		Args:  cobra.ExactArgs(1),
		RunE:  runBuild,
	}

	cmd.Flags().StringArrayVar(&boardIDs, "board", nil,
		fmt.Sprintf("board to build for (repeatable); omit to build all boards: %s", strings.Join(boards.IDs(), ", ")))
	cmd.Flags().StringVarP(&output, "output", "o", "",
		"output .img file when building one board, or output directory when building several")
	cmd.Flags().StringVar(&hostname, "hostname", "",
		"device hostname (default: sanitized main package name); an explicit value is baked into gosd.toml and always wins, while the default is left commented out so an Imager wizard hostname can take effect instead")
	cmd.Flags().StringVar(&wifiSSID, "wifi-ssid", "", "WiFi SSID to bake into the image")
	cmd.Flags().StringVar(&wifiPass, "wifi-pass", "", "WiFi password to bake into the image (WPA2-PSK or open networks only)")
	cmd.Flags().StringVar(&artifactsDir, "artifacts-dir", "",
		"directory of local kernel/firmware/bootloader files, checked before falling back to a pinned-URL download")
	cmd.Flags().StringVar(&gosdInitSrc, "gosd-init-src", os.Getenv("GOSD_INIT_SRC"),
		"directory containing gosd-init's main package source; overrides gosd's normal detection (dev checkout, then module cache) for unusual setups (default: $GOSD_INIT_SRC, the hook package managers use to point at their bundled copy)")
	cmd.Flags().StringVar(&dataSize, "data-size", defaultDataSize,
		"size of the writable GOSD-DATA partition (e.g. 512MiB, 2GiB), or 'expand' to keep the image small and have the device create the partition on first boot, filling the rest of the card; default 0 omits the partition entirely, so persistent /data is opt-in")
	cmd.Flags().StringVar(&bootSize, "boot-size", defaultBootSize,
		"size of the FAT32 GOSD-BOOT partition (e.g. 512MiB, 2GiB); default 256MiB fits every stock board's kernel/initramfs, but a large app may need more - the build fails with an actionable error naming this flag if it doesn't fit; this size becomes part of the app's on-disk layout, so changing it in a later release erases GOSD-DATA on upgrade (see docs/design/upgrade-path.md §0.4)")
	cmd.Flags().BoolVar(&catalogFlag, "catalog", false,
		"also emit a Raspberry Pi Imager custom-repository os_list.json (per image, plus a combined file) alongside the built image(s); requires --publish-base-url")
	cmd.Flags().StringVar(&publishBaseURL, "publish-base-url", "",
		"base URL the built image(s) will be hosted at, used to build the catalog's download links; required by --catalog")
	cmd.Flags().BoolVar(&usbGadget, "usb-gadget", false,
		"boot the board's USB port in peripheral mode, required if your app uses the gadget package (on the Pi Zero 2W this repurposes its only USB port from host to peripheral mode; no effect on Radxa Zero 3E)")
	cmd.Flags().StringArrayVar(&envFlags, "env", nil,
		"default app environment variable KEY=VALUE to bake into the image (repeatable); a hand-edited gosd.toml [env] entry on the card overrides the same key")
	cmd.Flags().StringVar(&kernelCfgPath, "kernel-config", "",
		fmt.Sprintf("developer kernel overlay config, read for its [[firmware]] entries only (default: %s in the working directory, if present)", defaultKernelConfigFile))
	cmd.Flags().StringArrayVar(&withExternal, "with-external", nil,
		"prebuilt static executable to bundle into the image at <path>[:<dest>] (repeatable); dest must be absolute, default /bin/<basename of path>; the binary must be a fully static ELF matching each selected board's architecture")
	cmd.Flags().IntVar(&consoleBaud, "console-baud", 0,
		"override the serial console baud rate baked into the boot config (e.g. 115200); default: each board's own rate (1500000 on the Rockchip boards, 115200 on the Pi boards) - useful when a USB-serial adapter can't reliably read the default rate (see COMPATIBILITY.md); the UART device itself (ttyS2, etc.) is unaffected, only its rate")
	cmd.Flags().BoolVar(&dataFlush, "data-flush", false,
		"mount GOSD-DATA, and any emmc/disk vfat volume, with the vfat \"flush\" option, pushing a file's data and metadata to the card promptly on close(2); default false uses normal Linux writeback (~30s dirty_expire) for faster writes, which is fine for apps using the documented durable-write pattern (fsync+rename, see docs/runtime.md#making-a-write-durable) - flush trades that write speed for prompter (but still not durable on its own) writeback; override per-device with gosd.toml's data_flush key")
	cmd.Flags().StringArrayVar(&placeholders, "placeholder", nil,
		"reserve a fixed-size comment-padded placeholder file on GOSD-BOOT at <path>=<size> (e.g. --placeholder backupist.yaml=32KiB, repeatable) and write a <image>.inject.json manifest beside each built image recording the absolute byte ranges a provisioning tool can overwrite with same-length bytes in the downloaded .img without any FAT tooling; see docs/image-injection.md")

	return cmd
}

func runBuild(cmd *cobra.Command, args []string) error {
	pkgPath := args[0]

	if catalogFlag && publishBaseURL == "" {
		return fmt.Errorf("--catalog requires --publish-base-url=<https://...> so the generated os_list.json can build download links; try e.g. --publish-base-url=https://example.com/downloads")
	}

	env, err := parseEnvFlags(envFlags)
	if err != nil {
		return err
	}

	externalSpecs, err := parseWithExternalFlags(withExternal)
	if err != nil {
		return err
	}

	dataSizeBytes, dataExpand, err := parseDataSize(dataSize)
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

	if err := validateConsoleBaud(selected, consoleBaud); err != nil {
		return err
	}

	appName, err := deriveAppName(pkgPath)
	if err != nil {
		return err
	}
	hostnameExplicit := hostname != ""
	deviceHostname := hostname
	if deviceHostname == "" {
		deviceHostname = appName
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

	binaries, err := compileForBoards(selected, tempDir, pkgPath, gosdInitSrc, build.CrossCompile, build.CrossCompileGosdInit)
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

		opts := pipeline.Options{
			Board:          b,
			AppBinaryPath:  bin.appPath,
			InitBinaryPath: bin.initPath,
			Config: boards.BuildConfig{
				Hostname:         deviceHostname,
				HostnameExplicit: hostnameExplicit,
				WifiSSID:         wifiSSID,
				WifiPassword:     wifiPass,
				UsbGadget:        usbGadget,
				Env:              env,
				ConsoleBaud:      consoleBaud,
			},
			ArtifactsDir:     artifactsDir,
			CacheDir:         cacheDir,
			OutputPath:       outputs[b.Name()],
			DataSizeBytes:    dataSizeBytes,
			DataExpand:       dataExpand,
			DataFlush:        dataFlush,
			BootSizeBytes:    bootSizeBytes,
			ExtraFirmware:    extraFirmware,
			ExtraExecutables: extraExecutables,
			Placeholders:     placeholderSpecs,
		}
		report, err := pipeline.Assemble(ctx, opts)
		if err != nil {
			if errors.Is(err, image.ErrBootPartitionFull) {
				return fmt.Errorf("building %s for %s failed: %w; pass a larger --boot-size than %s and rebuild", appName, b.Name(), err, humanizeBinaryBytes(bootSizeBytes))
			}
			return fmt.Errorf("building %s for %s failed: %w", appName, b.Name(), err)
		}
		printBootVolumeUsage(cmd, b.Name(), report)

		if len(placeholderSpecs) > 0 {
			manifestPath, err := inject.WriteManifest(outputs[b.Name()], b.Name(), placeholderSpecs, report.FileRanges)
			if err != nil {
				return fmt.Errorf("writing the injection manifest for %s (%s) failed: %w", appName, b.Name(), err)
			}
			cmd.PrintErrf("gosd build: %s inject manifest: %s\n", b.Name(), manifestPath)
		}
	}

	if catalogFlag {
		if err := writeCatalog(cmd, selected, appName, outputs); err != nil {
			return err
		}
	}

	return nil
}

// deriveAppName computes the default app name gosd uses for the device
// hostname and output filenames: the sanitized basename of pkgPath's
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
// partition. A size past the largest FAT32 volume GoSD can write is refused
// here, before any image bytes exist, rather than after a long build produces
// a silently corrupt partition.
func parseDataSize(s string) (bytes int64, expand bool, err error) {
	trimmed := strings.TrimSpace(s)
	if strings.EqualFold(trimmed, "expand") {
		return 0, true, nil
	}

	size, err := parseSizeBytes("--data-size", s)
	if err != nil {
		return 0, false, fmt.Errorf("%w, 'expand' to fill the card on first boot, or 0 to disable the data partition", err)
	}
	if size > diskfmt.MaxFAT32Bytes() {
		return 0, false, fmt.Errorf("--data-size %q is larger than GoSD can format: the largest GOSD-DATA partition it will create is %s (%d bytes), because %s; use --data-size=256GiB or less (--data-size=%d for the exact maximum), or --data-size=expand to fill the card up to 256GiB on first boot; if the app needs more storage than that, attach a disk and format it exFAT with the disk package, which has no such ceiling - see %s",
			s, diskfmt.GibibytesString(diskfmt.MaxFAT32Bytes()), diskfmt.MaxFAT32Bytes(), diskfmt.FAT32SizeLimitReason, diskfmt.MaxFAT32Bytes(), dataSizeLimitDocsURL)
	}
	return size, false, nil
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
		return 0, fmt.Errorf("--boot-size %q is larger than GoSD can format: the largest GOSD-BOOT partition it will create is %s (%d bytes), because %s",
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
// size can't contain one; splitting on the first keeps this consistent with
// --env's strings.Cut), sized via parseSizeBytes (the same binary-unit
// parser --data-size/--boot-size share), and validated (path shape,
// minimum/maximum size - see inject.Placeholder.Validate) before any
// compilation starts, so a bad --placeholder fails fast. A path given more
// than once, case-insensitively (FAT paths are case-insensitive), is
// rejected outright - the same "duplicate is more likely a mistake than
// intent" call parseEnvFlags makes for --env.
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

// envKeyPattern is the shape a --env KEY must match: a POSIX-ish environment
// variable name, the same rules gosd-init and any shell/exec environment
// already expect.
var envKeyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// parseEnvFlags turns the repeated --env KEY=VALUE flag values into the map
// baked into config.json and rendered into gosd.toml [env] (see
// boards.BuildConfig.Env). Only the first "=" splits key from value, so
// VALUE may be empty and may itself contain "=". A KEY given more than once
// across repeated --env flags is rejected outright, rather than letting the
// last one silently win — a mistaken duplicate is far more likely than an
// intentional override, and the intentional case still has a clear fix
// (remove one of the flags).
func parseEnvFlags(flags []string) (map[string]string, error) {
	if len(flags) == 0 {
		return nil, nil
	}

	env := make(map[string]string, len(flags))
	for _, flag := range flags {
		key, value, ok := strings.Cut(flag, "=")
		if !ok {
			return nil, fmt.Errorf("--env needs KEY=VALUE; got %q", flag)
		}

		if !envKeyPattern.MatchString(key) {
			return nil, fmt.Errorf("--env key %q is invalid because it doesn't match [A-Za-z_][A-Za-z0-9_]*; try renaming it to use only letters, digits and underscores, and not start with a digit", key)
		}
		if strings.HasPrefix(key, "GOSD_") {
			return nil, fmt.Errorf("--env %s is invalid because GOSD_* names are reserved by gosd; rename %s", key, key)
		}
		if _, dup := env[key]; dup {
			return nil, fmt.Errorf("--env %s was passed more than once; remove the duplicate --env flag or pick a different key for one of them", key)
		}
		env[key] = value
	}
	return env, nil
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
func writeCatalog(cmd *cobra.Command, selected []boards.Board, appName string, outputs map[string]string) error {
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
	if _, err := catalog.WriteFiles(dir, images, catalog.Options{BaseURL: publishBaseURL}); err != nil {
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
