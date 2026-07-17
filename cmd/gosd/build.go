package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jphastings/gosd/internal/boards"
	"github.com/jphastings/gosd/internal/boards/nanopizero2"
	"github.com/jphastings/gosd/internal/boards/pizero2w"
	"github.com/jphastings/gosd/internal/boards/pizerow"
	"github.com/jphastings/gosd/internal/boards/qemuvirt"
	"github.com/jphastings/gosd/internal/boards/radxazero3e"
	"github.com/jphastings/gosd/internal/boards/rock4se"
	"github.com/jphastings/gosd/internal/build"
	"github.com/jphastings/gosd/internal/catalog"
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
	catalogFlag    bool
	publishBaseURL string
	usbGadget      bool
	envFlags       []string
	kernelCfgPath  string
	withExternal   []string
)

// defaultDataSize is the GOSD-DATA partition size used when --data-size is
// not given. It defaults to 0 (no data partition): persistence is opt-in, so
// appliance images that don't need /data don't pay its image-size and
// flash-time cost.
const defaultDataSize = "0"

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
		"device hostname (default: sanitized main package name)")
	cmd.Flags().StringVar(&wifiSSID, "wifi-ssid", "", "WiFi SSID to bake into the image")
	cmd.Flags().StringVar(&wifiPass, "wifi-pass", "", "WiFi password to bake into the image (WPA2-PSK or open networks only)")
	cmd.Flags().StringVar(&artifactsDir, "artifacts-dir", "",
		"directory of local kernel/firmware/bootloader files, checked before falling back to a pinned-URL download")
	cmd.Flags().StringVar(&gosdInitSrc, "gosd-init-src", os.Getenv("GOSD_INIT_SRC"),
		"directory containing gosd-init's main package source; overrides gosd's normal detection (dev checkout, then module cache) for unusual setups (default: $GOSD_INIT_SRC, the hook package managers use to point at their bundled copy)")
	cmd.Flags().StringVar(&dataSize, "data-size", defaultDataSize,
		"size of the writable GOSD-DATA partition (e.g. 512MiB, 2GiB); default 0 omits the partition entirely, so persistent /data is opt-in")
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

	selected, err := resolveBoards(boardIDs)
	if err != nil {
		return err
	}

	appName := naming.Sanitize(filepath.Base(filepath.Clean(pkgPath)))
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

	dataSizeBytes, err := parseDataSize(dataSize)
	if err != nil {
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
				Hostname:     deviceHostname,
				WifiSSID:     wifiSSID,
				WifiPassword: wifiPass,
				UsbGadget:    usbGadget,
				Env:          env,
			},
			ArtifactsDir:     artifactsDir,
			CacheDir:         cacheDir,
			OutputPath:       outputs[b.Name()],
			DataSizeBytes:    dataSizeBytes,
			ExtraFirmware:    extraFirmware,
			ExtraExecutables: extraExecutables,
		}
		if err := pipeline.Assemble(ctx, opts); err != nil {
			return fmt.Errorf("building %s for %s failed: %w", appName, b.Name(), err)
		}
	}

	if catalogFlag {
		if err := writeCatalog(cmd, selected, appName, outputs); err != nil {
			return err
		}
	}

	return nil
}

// dataSizeUnits are the size suffixes --data-size accepts, all binary
// (power-of-1024) units: partition sizes are conventionally binary, and
// offering only one interpretation avoids MB-vs-MiB ambiguity.
var dataSizeUnits = map[string]int64{
	"KIB": 1024,
	"MIB": 1024 * 1024,
	"GIB": 1024 * 1024 * 1024,
	"K":   1024,
	"M":   1024 * 1024,
	"G":   1024 * 1024 * 1024,
}

// parseDataSize parses a --data-size value like "512MiB", "2G", or "0" into
// bytes. A bare number is bytes; 0 (with or without a unit) disables the
// data partition.
func parseDataSize(s string) (int64, error) {
	trimmed := strings.TrimSpace(s)
	numPart := trimmed
	var multiplier int64 = 1
	for suffix, mult := range dataSizeUnits {
		if n, ok := strings.CutSuffix(strings.ToUpper(trimmed), suffix); ok {
			numPart, multiplier = strings.TrimSpace(n), mult
			break
		}
	}

	n, err := strconv.ParseInt(numPart, 10, 64)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("--data-size %q is not a valid size; use a number with a binary unit (e.g. 512MiB, 1GiB) or 0 to disable the data partition", s)
	}
	if multiplier > 1 && n > math.MaxInt64/multiplier {
		return 0, fmt.Errorf("--data-size %q is too large; choose something that fits on an SD card", s)
	}
	return n * multiplier, nil
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
