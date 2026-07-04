// Package pipeline wires the pieces gosd build needs into one flashable
// image per board: resolving pinned/local artifacts, building the
// initramfs (app + gosd-init + firmware + config.json), asking the board
// profile for its boot files and raw writes, and writing the finished .img
// via internal/image.
package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/jphastings/gosd/internal/boards"
	"github.com/jphastings/gosd/internal/gosdtoml"
	"github.com/jphastings/gosd/internal/image"
	"github.com/jphastings/gosd/internal/initcfg"
	"github.com/jphastings/gosd/internal/initramfs"
)

const (
	initFileMode     = 0o755
	appFileMode      = 0o755
	configFileMode   = 0o644
	firmwareFileMode = 0o644
)

// Options describes everything needed to assemble one board's image from a
// full gosd build: the cross-compiled binaries, the board to build for, and
// where its artifacts and finished image live.
type Options struct {
	Board boards.Board

	// AppBinaryPath is the cross-compiled user application.
	AppBinaryPath string
	// InitBinaryPath is the cross-compiled gosd-init binary.
	InitBinaryPath string

	// Config is the per-build configuration baked into
	// /etc/gosd/config.json (hostname, WiFi) and passed through to the
	// board's BootFiles.
	Config boards.BuildConfig

	// ArtifactsDir is checked for each of Board.Artifacts() by name
	// before falling back to a pinned-URL fetch into CacheDir. Pointing
	// it at a directory that already contains every artifact a board
	// needs (as gosd's integration tests do) means the build never
	// touches the network.
	ArtifactsDir string
	// CacheDir is where artifacts fetched from a pinned URL are cached
	// across builds.
	CacheDir string

	// OutputPath is where the finished .img file is written.
	OutputPath string
}

// Assemble runs the full build pipeline for one board: resolve artifacts,
// build the initramfs, ask the board for its boot files and raw writes, and
// write the resulting flashable image to opts.OutputPath.
func Assemble(ctx context.Context, opts Options) error {
	artifacts, err := boards.ResolveArtifacts(ctx, opts.Board.Artifacts(), opts.ArtifactsDir, opts.CacheDir)
	if err != nil {
		return fmt.Errorf("resolving artifacts for %s: %w", opts.Board.Name(), err)
	}

	firmware := opts.Board.FirmwareFiles(artifacts)
	defer closeReaders(firmware)

	initBin, err := os.Open(opts.InitBinaryPath)
	if err != nil {
		return fmt.Errorf("opening gosd-init binary at %s: %w", opts.InitBinaryPath, err)
	}
	defer func() { _ = initBin.Close() }()

	appBin, err := os.Open(opts.AppBinaryPath)
	if err != nil {
		return fmt.Errorf("opening app binary at %s: %w", opts.AppBinaryPath, err)
	}
	defer func() { _ = appBin.Close() }()

	configJSON, err := json.Marshal(initcfg.Config{
		Board:    opts.Board.Name(),
		Hostname: opts.Config.Hostname,
		Wifi: initcfg.Wifi{
			SSID:       opts.Config.WifiSSID,
			Passphrase: opts.Config.WifiPassword,
		},
	})
	if err != nil {
		return fmt.Errorf("encoding config.json for %s: %w", opts.Board.Name(), err)
	}

	files := make([]initramfs.File, 0, len(firmware)+3)
	files = append(files,
		initramfs.File{Path: "/init", Content: initBin, Mode: initFileMode},
		initramfs.File{Path: "/app", Content: appBin, Mode: appFileMode},
		initramfs.File{Path: "/etc/gosd/config.json", Content: bytes.NewReader(configJSON), Mode: configFileMode},
	)
	for name, r := range firmware {
		files = append(files, initramfs.File{Path: "/lib/firmware/" + name, Content: r, Mode: firmwareFileMode})
	}

	var initramfsBuf bytes.Buffer
	if err := initramfs.Build(&initramfsBuf, initramfs.Spec{Files: files}); err != nil {
		return fmt.Errorf("building the initramfs for %s: %w", opts.Board.Name(), err)
	}
	artifacts.Initramfs = &initramfsBuf

	bootFiles, err := opts.Board.BootFiles(opts.Config, artifacts)
	if err != nil {
		return fmt.Errorf("assembling boot files for %s: %w", opts.Board.Name(), err)
	}
	if bootFiles == nil {
		bootFiles = make(map[string]io.Reader, 1)
	}
	defer closeReaders(bootFiles)

	// gosd.toml is common to every board (unlike config.txt/extlinux.conf,
	// which are board-specific), so it's added here rather than inside any
	// Board.BootFiles implementation: both boards get it at the FAT root.
	bootFiles["gosd.toml"] = bytes.NewReader(gosdtoml.Render(opts.Config.Hostname, opts.Config.WifiSSID, opts.Config.WifiPassword))

	if err := image.Write(opts.OutputPath, image.Spec{
		BootFiles: bootFiles,
		RawWrites: opts.Board.RawWrites(artifacts),
	}); err != nil {
		return fmt.Errorf("writing the image for %s to %s: %w", opts.Board.Name(), opts.OutputPath, err)
	}

	return nil
}

// closeReaders best-effort-closes any reader in files that also implements
// io.Closer (e.g. the *os.File values Artifacts.Open returns).
func closeReaders(files map[string]io.Reader) {
	for _, r := range files {
		if c, ok := r.(io.Closer); ok {
			_ = c.Close()
		}
	}
}
