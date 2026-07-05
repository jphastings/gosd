package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jphastings/gosd/internal/boards"
	"github.com/jphastings/gosd/internal/boards/pizero2w"
	"github.com/jphastings/gosd/internal/boards/radxazero3e"
	"github.com/jphastings/gosd/internal/build"
	"github.com/jphastings/gosd/internal/naming"
	"github.com/jphastings/gosd/internal/pipeline"
)

func init() {
	boards.Register(pizero2w.New())
	boards.Register(radxazero3e.New())
}

var (
	boardIDs     []string
	output       string
	hostname     string
	wifiSSID     string
	wifiPass     string
	artifactsDir string
	gosdInitSrc  string
)

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
	cmd.Flags().StringVar(&gosdInitSrc, "gosd-init-src", "",
		"directory containing gosd-init's main package source; overrides gosd's normal detection (dev checkout, then module cache) for unusual setups")

	return cmd
}

func runBuild(cmd *cobra.Command, args []string) error {
	pkgPath := args[0]

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

	tempDir, err := os.MkdirTemp("", "gosd-build-")
	if err != nil {
		return fmt.Errorf("creating a temp build directory failed: %w", err)
	}

	appBinary := filepath.Join(tempDir, appName)
	if err := build.CrossCompile(pkgPath, appBinary); err != nil {
		return fmt.Errorf("cross-compiling %s failed: %w", pkgPath, err)
	}

	initBinary := filepath.Join(tempDir, "gosd-init")
	if err := build.CrossCompileGosdInit(initBinary, gosdInitSrc); err != nil {
		return fmt.Errorf("cross-compiling gosd-init failed: %w", err)
	}

	ctx := cmd.Context()
	cacheDir, err := artifactCacheDir()
	if err != nil {
		return err
	}

	for _, b := range selected {
		opts := pipeline.Options{
			Board:          b,
			AppBinaryPath:  appBinary,
			InitBinaryPath: initBinary,
			Config: boards.BuildConfig{
				Hostname:     deviceHostname,
				WifiSSID:     wifiSSID,
				WifiPassword: wifiPass,
			},
			ArtifactsDir: artifactsDir,
			CacheDir:     cacheDir,
			OutputPath:   outputs[b.Name()],
		}
		if err := pipeline.Assemble(ctx, opts); err != nil {
			return fmt.Errorf("building %s for %s failed: %w", appName, b.Name(), err)
		}
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
