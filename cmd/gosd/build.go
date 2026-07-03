package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jphastings/gosd/internal/boards"
	"github.com/jphastings/gosd/internal/build"
	"github.com/jphastings/gosd/internal/image"
	"github.com/jphastings/gosd/internal/initramfs"
	"github.com/jphastings/gosd/internal/naming"
)

// gosdInitPkg is the import path gosd cross-compiles for the init binary
// baked into every image. Building it via an absolute import path (rather
// than a path relative to the caller's working directory) only works today
// because gosd is run from within its own module; packaging/distributing
// gosd for use outside this repo is a separate, later concern.
const gosdInitPkg = "github.com/jphastings/gosd/cmd/gosd-init"

var (
	boardIDs []string
	output   string
	hostname string
	wifiSSID string
	wifiPass string
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
	if err := build.CrossCompile(gosdInitPkg, initBinary); err != nil {
		return fmt.Errorf("cross-compiling gosd-init failed: %w", err)
	}

	ctx := cmd.Context()
	var assembler image.Assembler = image.NotImplemented{}
	var initramfsBuilder initramfs.Builder = initramfs.NotImplemented{}

	for _, b := range selected {
		spec := image.Spec{
			Board:          b,
			AppBinaryPath:  appBinary,
			InitBinaryPath: initBinary,
			Initramfs:      initramfsBuilder,
			Hostname:       deviceHostname,
			WifiSSID:       wifiSSID,
			WifiPassword:   wifiPass,
			OutputPath:     outputs[b.ID],
		}
		if err := assembler.Assemble(ctx, spec); err != nil {
			return fmt.Errorf("building %s for %s failed: %w", appName, b.ID, err)
		}
	}

	return nil
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
			path = fmt.Sprintf("%s-%s.img", appName, b.ID)
		}
		outputs[b.ID] = path
		return outputs, nil
	}

	dir := output
	if dir == "" {
		dir = "."
	}
	for _, b := range selected {
		outputs[b.ID] = filepath.Join(dir, fmt.Sprintf("%s-%s.img", appName, b.ID))
	}
	return outputs, nil
}
