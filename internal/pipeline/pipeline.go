// Package pipeline wires the pieces gosd build needs into one flashable
// image per board: resolving pinned/local artifacts, building the
// initramfs (app + gosd-init + firmware + config.json), computing
// config.json's content-derived image identity over that same payload
// (see internal/initcfg.ComputeIdentity), asking the board profile for its
// boot files and raw writes, and writing the finished .img via
// internal/image.
package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/jphastings/gosd/internal/artifacts"
	"github.com/jphastings/gosd/internal/boards"
	"github.com/jphastings/gosd/internal/gosdtoml"
	"github.com/jphastings/gosd/internal/hostsfile"
	"github.com/jphastings/gosd/internal/image"
	"github.com/jphastings/gosd/internal/initcfg"
	"github.com/jphastings/gosd/internal/initramfs"
	"github.com/jphastings/gosd/internal/inject"
)

const (
	initFileMode       = 0o755
	appFileMode        = 0o755
	configFileMode     = 0o644
	firmwareFileMode   = 0o644
	executableFileMode = 0o755
	hostsFileMode      = 0o644
	extraFileMode      = 0o644
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

	// ExtraFiles holds additional non-executable files to land at their
	// given absolute dest inside the initramfs, at mode 0644 rather than
	// ExtraExecutables' 0755 - the right mode for data such as the
	// Mozilla CA bundle (gosd build/run, bean gosd-kzgq), keyed by dest,
	// mirroring ExtraExecutables' shape in every other respect including
	// close discipline: Assemble closes every reader once it's done with
	// it.
	ExtraFiles map[string]io.Reader

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

	// DataExpand marks an image built with --data-size=expand: the image
	// itself gets no data partition (DataSizeBytes stays 0), and
	// config.json tells gosd-init to create one filling the rest of the
	// card on first boot.
	DataExpand bool

	// DataFlush is gosd build --data-flush's value, baked straight into
	// config.json's DataFlush field: whether GOSD-DATA (and any emmc/disk
	// vfat mount) uses the vfat "flush" mount option by default. Default
	// false (see internal/initcfg.Config.DataFlush and bean gosd-9m1k);
	// overridable per-device via gosd.toml's data_flush key at boot, which
	// is why this is a plain baked default and not a template value like
	// Config.Env — gosd-init computes the effective setting itself.
	DataFlush bool

	// IngressCloudflared is `gosd build --ingress cloudflared`'s value,
	// baked straight into config.json's IngressCloudflared field (see
	// initcfg.Config.IngressCloudflared's doc comment for the full
	// build->runtime contract). cmd/gosd is responsible for actually
	// putting the cloudflared binary into ExtraExecutables at
	// /bin/cloudflared - this field only carries the "is it baked" bit
	// through to config.json, exactly like DataFlush does for its own
	// flag.
	IngressCloudflared bool

	// IngressTailscaleFunnel is `gosd build --ingress tailscale-funnel`'s
	// value, baked straight into config.json's IngressTailscaleFunnel field
	// (see initcfg.Config.IngressTailscaleFunnel's doc comment). Mirrors
	// IngressCloudflared exactly: cmd/gosd is responsible for actually
	// putting the compiled shim into ExtraExecutables at
	// /bin/gosd-tsfunnel - this field only carries the "is it baked" bit
	// through to config.json.
	IngressTailscaleFunnel bool

	// BootSizeBytes is the size of the FAT32 GOSD-BOOT partition, passed
	// straight through to image.Spec.BootSizeBytes. Zero means
	// image.DefaultBootPartitionSizeBytes (256MiB).
	BootSizeBytes int64

	// Placeholders are `gosd build --placeholder <path>=<size>` entries:
	// rendered deterministically (see inject.Render), they land at the
	// FAT root of GOSD-BOOT alongside gosd.toml, are covered by the image
	// identity exactly like every other FAT-root file, and their content
	// byte ranges are reported back in the image.WriteReport's
	// FileRanges - the raw material for cmd/gosd's <image>.inject.json
	// sidecar (see internal/inject.WriteManifest).
	Placeholders []inject.Placeholder
}

// Assemble runs the full build pipeline for one board: resolve artifacts,
// build the initramfs, ask the board for its boot files and raw writes, and
// write the resulting flashable image to opts.OutputPath. The returned
// image.WriteReport lets a caller print a boot-volume usage summary.
func Assemble(ctx context.Context, opts Options) (image.WriteReport, error) {
	resolved, err := boards.ResolveArtifacts(ctx, opts.Board.Name(), opts.Board.Artifacts(), opts.ArtifactsDir, opts.CacheDir, fetchBoardArtifacts)
	if err != nil {
		return image.WriteReport{}, fmt.Errorf("resolving artifacts for %s: %w", opts.Board.Name(), err)
	}

	firmware := opts.Board.FirmwareFiles(resolved)
	for name, r := range opts.ExtraFirmware {
		firmware[name] = r
	}
	defer closeReaders(firmware)
	defer closeReaders(opts.ExtraExecutables)
	defer closeReaders(opts.ExtraFiles)

	// Every input that contributes to config.json's image identity (see
	// initcfg.ComputeIdentity) has to be read into memory before it can be
	// hashed, so everything below is read exactly once, here, and the same
	// bytes are reused for both hashing and the real archive/partition
	// writes further down — nothing is read from disk (or decompressed)
	// twice.
	initBinBytes, err := os.ReadFile(opts.InitBinaryPath)
	if err != nil {
		return image.WriteReport{}, fmt.Errorf("opening gosd-init binary at %s: %w", opts.InitBinaryPath, err)
	}
	appBinBytes, err := os.ReadFile(opts.AppBinaryPath)
	if err != nil {
		return image.WriteReport{}, fmt.Errorf("opening app binary at %s: %w", opts.AppBinaryPath, err)
	}
	firmwareBytes, err := readAllReaders(firmware)
	if err != nil {
		return image.WriteReport{}, fmt.Errorf("reading firmware files for %s: %w", opts.Board.Name(), err)
	}
	extraExecBytes, err := readAllReaders(opts.ExtraExecutables)
	if err != nil {
		return image.WriteReport{}, fmt.Errorf("reading extra executables for %s: %w", opts.Board.Name(), err)
	}
	extraFileBytes, err := readAllReaders(opts.ExtraFiles)
	if err != nil {
		return image.WriteReport{}, fmt.Errorf("reading extra files for %s: %w", opts.Board.Name(), err)
	}

	// Board.BootFiles requires a non-nil Initramfs even though it never
	// reads it — it just threads the reader through to its returned map,
	// opaquely, under its own well-known name (every board's BootFiles
	// does exactly this; see e.g. internal/boards/pizero2w). The real
	// archive can't exist yet: building it needs the final config.json,
	// and config.json's Identity needs everything else BootFiles is about
	// to return (kernel, DTB, the board's boot-config file...). So a
	// placeholder stands in for this one call, purely to satisfy that
	// nil-check; initramfsPlaceholder's identity (not its — empty —
	// content) is what finds the right map key to overwrite with the real
	// archive once it exists, below.
	initramfsPlaceholder := &bytes.Buffer{}
	resolved.Initramfs = initramfsPlaceholder

	bootFiles, err := opts.Board.BootFiles(opts.Config, resolved)
	if err != nil {
		return image.WriteReport{}, fmt.Errorf("assembling boot files for %s: %w", opts.Board.Name(), err)
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
	// see and override. It's added before the read-and-hash loop below so
	// it's covered by the image identity like every other FAT-root file.
	//
	// The hostname line is only baked in uncommented when the developer
	// explicitly chose it (opts.Config.HostnameExplicit); the default
	// (sanitized main-package name) renders commented, like [wifi], so an
	// Imager wizard's cloud-init hostname isn't always shadowed by it (see
	// bean gosd-4hz1 and gosdtoml.Render's docstring).
	//
	// The zero Ingress value below always renders the commented example:
	// gosd build only ever bakes the cloudflared binary in (config.json's
	// ingressCloudflared bit), never a real token/hostname/port — that's a
	// per-device secret nothing at build time could supply — so there's
	// never a real value to bake here.
	bootFiles["gosd.toml"] = bytes.NewReader(gosdtoml.Render(opts.Config.Hostname, opts.Config.HostnameExplicit, opts.Config.WifiSSID, opts.Config.WifiPassword, opts.Config.Env, gosdtoml.Ingress{}))

	// opts.Placeholders land at the FAT root the same way, right after
	// gosd.toml and still before the read-and-hash loop below, so they're
	// covered by the image identity exactly like every other FAT-root
	// file. FAT is case-insensitive, so a placeholder path colliding with
	// any existing boot file (gosd.toml included) or an earlier
	// placeholder is refused case-insensitively — two directory entries
	// differing only by case can't coexist on the card anyway. Checking
	// against bootFiles itself (mutated as the loop adds each rendered
	// placeholder) catches both kinds of collision with one check.
	reportRanges := make([]string, 0, len(opts.Placeholders))
	for _, p := range opts.Placeholders {
		for existing := range bootFiles {
			if strings.EqualFold(existing, p.Path) {
				return image.WriteReport{}, fmt.Errorf("--placeholder %s collides with an existing boot file %q (FAT paths are case-insensitive); choose a different --placeholder path", p.Path, existing)
			}
		}
		rendered, err := inject.Render(p)
		if err != nil {
			return image.WriteReport{}, fmt.Errorf("rendering --placeholder %s failed: %w", p.Path, err)
		}
		bootFiles[p.Path] = bytes.NewReader(rendered)
		reportRanges = append(reportRanges, p.Path)
	}

	// Read every FAT-root file into memory — both to hash it into the
	// image identity below and to serve image.Write from a fresh reader
	// later (the originals, e.g. artifact files opened by Board.BootFiles,
	// are each read to EOF and closed here). initramfsKey is remembered,
	// not read: its reader is the placeholder from above, standing in
	// until the real archive is built.
	var initramfsKey string
	payload := make([]initcfg.PayloadFile, 0, len(bootFiles)+len(firmwareBytes)+len(extraExecBytes)+len(extraFileBytes)+2)
	for name, r := range bootFiles {
		if r == io.Reader(initramfsPlaceholder) {
			initramfsKey = name
			continue
		}
		data, err := readAllAndClose(r)
		if err != nil {
			return image.WriteReport{}, fmt.Errorf("reading boot file %q for %s: %w", name, opts.Board.Name(), err)
		}
		bootFiles[name] = bytes.NewReader(data)
		payload = append(payload, initcfg.PayloadFile{Path: name, Content: data})
	}
	if initramfsKey == "" {
		return image.WriteReport{}, fmt.Errorf("assembling boot files for %s: BootFiles did not include the initramfs archive", opts.Board.Name())
	}

	payload = append(payload,
		initcfg.PayloadFile{Path: initcfg.InitramfsPayloadPath("/init"), Content: initBinBytes},
		initcfg.PayloadFile{Path: initcfg.InitramfsPayloadPath("/app"), Content: appBinBytes},
		initcfg.PayloadFile{Path: initcfg.InitramfsPayloadPath(hostsfile.Path), Content: []byte(hostsfile.Static())},
	)
	for name, data := range firmwareBytes {
		payload = append(payload, initcfg.PayloadFile{Path: initcfg.InitramfsPayloadPath("/lib/firmware/" + name), Content: data})
	}
	for dest, data := range extraExecBytes {
		payload = append(payload, initcfg.PayloadFile{Path: initcfg.InitramfsPayloadPath(dest), Content: data})
	}
	for dest, data := range extraFileBytes {
		payload = append(payload, initcfg.PayloadFile{Path: initcfg.InitramfsPayloadPath(dest), Content: data})
	}

	// config.json (/etc/gosd/config.json inside the initramfs) is
	// deliberately not part of payload — see ComputeIdentity's docstring
	// for why it can't be, and what that means Identity does and doesn't
	// cover.
	//
	// payload is hashed from these pre-FAT-write bytes rather than from
	// the finished .img, and that's load-bearing, not just convenient:
	// gosd build's own output isn't fully byte-reproducible today —
	// go-diskfs's FAT32 formatter stamps directory-entry timestamps and a
	// volume serial number from wall-clock time, confirmed by building the
	// same inputs twice and diffing the two images (a couple dozen bytes
	// differ, confined to exactly those fields). Hashing the files before
	// they reach the FAT layer hashes around that non-reproducible input
	// entirely, which is what TestBuildIdentityIsReproducibleAcrossRebuilds
	// (cmd/gosd/build_integration_test.go) checks.
	identity := initcfg.ComputeIdentity(payload)

	configJSON, err := json.Marshal(initcfg.Config{
		Board:    opts.Board.Name(),
		Hostname: opts.Config.Hostname,
		Wifi: initcfg.Wifi{
			SSID:       opts.Config.WifiSSID,
			Passphrase: opts.Config.WifiPassword,
		},
		Env:                    opts.Config.Env,
		DataExpand:             opts.DataExpand,
		DataFlush:              opts.DataFlush,
		IngressCloudflared:     opts.IngressCloudflared,
		IngressTailscaleFunnel: opts.IngressTailscaleFunnel,
		Identity:               identity,
		// Wall-clock, taken here rather than threaded in via Options: it
		// must vary build-to-build (that's the whole point, as
		// timesync's clock floor), and config.json is excluded from
		// ComputeIdentity's payload entirely, so it can't move Identity
		// - see BuildTimestamp's doc.
		BuildTimestamp: time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		return image.WriteReport{}, fmt.Errorf("encoding config.json for %s: %w", opts.Board.Name(), err)
	}

	files := make([]initramfs.File, 0, len(firmwareBytes)+len(extraExecBytes)+len(extraFileBytes)+4)
	files = append(files,
		initramfs.File{Path: "/init", Content: bytes.NewReader(initBinBytes), Mode: initFileMode},
		initramfs.File{Path: "/app", Content: bytes.NewReader(appBinBytes), Mode: appFileMode},
		initramfs.File{Path: "/etc/gosd/config.json", Content: bytes.NewReader(configJSON), Mode: configFileMode},
		// The static localhost/loopback lines only; gosd-init appends its
		// own 127.0.1.1 <hostname> line once the hostname settles at boot
		// (see hostsfile.Write and cmd/gosd-init/internal/boot/sequence.go)
		// — the hostname isn't known here, and gosd.toml/cloud-init can
		// still change it after this image is built.
		initramfs.File{Path: hostsfile.Path, Content: strings.NewReader(hostsfile.Static()), Mode: hostsFileMode},
	)
	for name, data := range firmwareBytes {
		files = append(files, initramfs.File{Path: "/lib/firmware/" + name, Content: bytes.NewReader(data), Mode: firmwareFileMode})
	}
	for dest, data := range extraExecBytes {
		files = append(files, initramfs.File{Path: dest, Content: bytes.NewReader(data), Mode: executableFileMode})
	}
	for dest, data := range extraFileBytes {
		files = append(files, initramfs.File{Path: dest, Content: bytes.NewReader(data), Mode: extraFileMode})
	}

	var initramfsBuf bytes.Buffer
	if err := initramfs.Build(&initramfsBuf, initramfs.Spec{Files: files, Dirs: mountPointDirs}); err != nil {
		return image.WriteReport{}, fmt.Errorf("building the initramfs for %s: %w", opts.Board.Name(), err)
	}
	resolved.Initramfs = &initramfsBuf
	bootFiles[initramfsKey] = &initramfsBuf

	report, err := image.Write(opts.OutputPath, image.Spec{
		BootFiles:     bootFiles,
		RawWrites:     opts.Board.RawWrites(resolved),
		DataSizeBytes: opts.DataSizeBytes,
		BootSizeBytes: opts.BootSizeBytes,
		ReportRanges:  reportRanges,
	})
	if err != nil {
		return image.WriteReport{}, fmt.Errorf("writing the image for %s to %s: %w", opts.Board.Name(), opts.OutputPath, err)
	}

	return report, nil
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

// readAllAndClose fully reads r, then best-effort closes it if it
// implements io.Closer — the same close discipline closeReaders applies to
// a whole map, for a single reader consumed inline.
func readAllAndClose(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(r)
	if c, ok := r.(io.Closer); ok {
		_ = c.Close()
	}
	return data, err
}

// readAllReaders reads every entry of files fully into memory (see
// readAllAndClose), returning their bytes keyed the same way files was.
func readAllReaders(files map[string]io.Reader) (map[string][]byte, error) {
	out := make(map[string][]byte, len(files))
	for name, r := range files {
		data, err := readAllAndClose(r)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		out[name] = data
	}
	return out, nil
}
