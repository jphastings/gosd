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

	"github.com/jphastings/gosd/internal/artifacts"
	"github.com/jphastings/gosd/internal/boards"
	"github.com/jphastings/gosd/internal/gosdtoml"
	"github.com/jphastings/gosd/internal/image"
	"github.com/jphastings/gosd/internal/initcfg"
	"github.com/jphastings/gosd/internal/initramfs"
)

const (
	initFileMode       = 0o755
	appFileMode        = 0o755
	configFileMode     = 0o644
	firmwareFileMode   = 0o644
	executableFileMode = 0o755
)

// mountPointDirs are the directories gosd-init unconditionally mounts
// something onto during boot (see cmd/gosd-init/internal/boot/mounts.go's
// earlyMounts and sequence.go's MountBootPartition), on every board. The
// initramfs starts out containing nothing but what this package writes, so
// without these, mount(2) fails with ENOENT before gosd-init gets anywhere
// - they must exist in the archive even though nothing is ever written
// inside them from here.
var mountPointDirs = []string{"/dev", "/proc", "/sys", "/run", "/boot"}

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

	// ExtraFirmware holds additional runtime firmware files to land under
	// /lib/firmware in the initramfs, alongside the board's own
	// FirmwareFiles() - keyed the same way, by path relative to
	// /lib/firmware. This is how gosd-kernel.toml's [[firmware]] entries
	// (bean gosd-hkp7) reach the image: cmd/gosd fetches and opens them
	// before calling Assemble, so this package stays free of any
	// developer-config or network-fetch concerns. A key also present in
	// the board's own firmware is overridden by the entry here. Assemble
	// closes every reader once it's done with it, exactly like the
	// board's own firmware readers.
	ExtraFirmware map[string]io.Reader

	// ExtraExecutables holds additional prebuilt static executables to land
	// at their given absolute dest inside the initramfs (gosd build
	// --with-external, bean gosd-ig4h) - keyed by dest (e.g. "/bin/mpv"),
	// mirroring ExtraFirmware's shape. cmd/gosd validates each dest and
	// pre-flights each binary's ELF class/machine against the board's Arch
	// before Assemble ever sees it, so this package stays free of any
	// developer-config or ELF-inspection concerns - it just writes the
	// bytes at mode 0755. Assemble closes every reader once it's done with
	// it, exactly like ExtraFirmware.
	ExtraExecutables map[string]io.Reader

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

	// DataSizeBytes is the size of the optional writable GOSD-DATA
	// partition, passed straight through to image.Spec.DataSizeBytes.
	// Zero disables the partition.
	DataSizeBytes int64
}

// Assemble runs the full build pipeline for one board: resolve artifacts,
// build the initramfs, ask the board for its boot files and raw writes, and
// write the resulting flashable image to opts.OutputPath.
func Assemble(ctx context.Context, opts Options) error {
	resolved, err := boards.ResolveArtifacts(ctx, opts.Board.Name(), opts.Board.Artifacts(), opts.ArtifactsDir, opts.CacheDir, fetchBoardArtifacts)
	if err != nil {
		return fmt.Errorf("resolving artifacts for %s: %w", opts.Board.Name(), err)
	}

	firmware := opts.Board.FirmwareFiles(resolved)
	for name, r := range opts.ExtraFirmware {
		firmware[name] = r
	}
	defer closeReaders(firmware)
	defer closeReaders(opts.ExtraExecutables)

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
		Env: opts.Config.Env,
	})
	if err != nil {
		return fmt.Errorf("encoding config.json for %s: %w", opts.Board.Name(), err)
	}

	files := make([]initramfs.File, 0, len(firmware)+len(opts.ExtraExecutables)+3)
	files = append(files,
		initramfs.File{Path: "/init", Content: initBin, Mode: initFileMode},
		initramfs.File{Path: "/app", Content: appBin, Mode: appFileMode},
		initramfs.File{Path: "/etc/gosd/config.json", Content: bytes.NewReader(configJSON), Mode: configFileMode},
	)
	for name, r := range firmware {
		files = append(files, initramfs.File{Path: "/lib/firmware/" + name, Content: r, Mode: firmwareFileMode})
	}
	for dest, r := range opts.ExtraExecutables {
		files = append(files, initramfs.File{Path: dest, Content: r, Mode: executableFileMode})
	}

	var initramfsBuf bytes.Buffer
	if err := initramfs.Build(&initramfsBuf, initramfs.Spec{Files: files, Dirs: mountPointDirs}); err != nil {
		return fmt.Errorf("building the initramfs for %s: %w", opts.Board.Name(), err)
	}
	resolved.Initramfs = &initramfsBuf

	bootFiles, err := opts.Board.BootFiles(opts.Config, resolved)
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
	// The baked env (opts.Config.Env, from `gosd build --env`) is rendered
	// here too, so the card shows the developer's defaults for the user to
	// see and override.
	bootFiles["gosd.toml"] = bytes.NewReader(gosdtoml.Render(opts.Config.Hostname, opts.Config.WifiSSID, opts.Config.WifiPassword, opts.Config.Env))

	if err := image.Write(opts.OutputPath, image.Spec{
		BootFiles:     bootFiles,
		RawWrites:     opts.Board.RawWrites(resolved),
		DataSizeBytes: opts.DataSizeBytes,
	}); err != nil {
		return fmt.Errorf("writing the image for %s to %s: %w", opts.Board.Name(), opts.OutputPath, err)
	}

	return nil
}

// fetchBoardArtifacts is the boards.BoardArtifactsFunc every real build uses:
// download and cache the requested board's CI-built artifact release (see
// bean gosd-wtpa and internal/artifacts) with the default HTTP client.
func fetchBoardArtifacts(ctx context.Context, cacheDir, board string) (string, error) {
	return artifacts.EnsureBoard(ctx, nil, cacheDir, board)
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
