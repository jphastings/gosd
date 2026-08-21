package main

import (
	"bytes"
	"crypto/sha256"
	"debug/elf"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	diskfs "github.com/diskfs/go-diskfs"
	"github.com/diskfs/go-diskfs/disk"
	"github.com/diskfs/go-diskfs/partition/mbr"
	"github.com/klauspost/compress/zstd"
	"github.com/u-root/u-root/pkg/cpio"

	"github.com/jphastings/gosd/internal/boards"
	"github.com/jphastings/gosd/internal/cacerts"
	"github.com/jphastings/gosd/internal/configtree"
	"github.com/jphastings/gosd/internal/diskfmt"
	"github.com/jphastings/gosd/internal/image"
	"github.com/jphastings/gosd/internal/initcfg"
	"github.com/jphastings/gosd/internal/inject"
)

// helloBootLabel and helloDataLabel are the volume labels every test here
// gets by building ../../examples/hello with no --label-prefix: the app's
// own name, short enough to need no truncation (see
// internal/naming.LabelPrefix).
const (
	helloBootLabel = "hello-boot"
	helloDataLabel = "hello-data"
)

// caCertsFixtureContent is testdata/fake-artifacts/ca-certificates.crt's
// exact content: a few lines of fake PEM text, never the real ~186KB
// Mozilla bundle, so every build-integration test that touches
// --artifacts-dir stays fast and hermetic while still exercising the same
// code path a real fetch.ToDir download would.
const caCertsFixtureContent = "-----BEGIN CERTIFICATE-----\nfake fixture CA bundle, not a real Mozilla bundle\n-----END CERTIFICATE-----\n"

// assertCACertsBaked confirms records - a built image's decoded initramfs -
// carries the CA bundle every image ships (bean gosd-kzgq) at its standard
// path and mode, sourced here from the --artifacts-dir fixture rather than a
// real network fetch (the network tripwire in each caller proves that).
func assertCACertsBaked(t *testing.T, records []cpio.Record) {
	t.Helper()

	name := strings.TrimPrefix(cacerts.InitramfsPath, "/")
	rec, ok := findRecord(records, name)
	if !ok {
		t.Fatalf("initramfs is missing %q; got entries %v", name, recordNames(records))
	}
	if want := uint64(cpio.S_IFREG | 0o644); rec.Mode != want {
		t.Errorf("%s Mode = %#o, want %#o", name, rec.Mode, want)
	}
	if got := string(recordContent(t, records, name)); got != caCertsFixtureContent {
		t.Errorf("%s content = %q, want %q", name, got, caCertsFixtureContent)
	}
}

// roundTripFunc adapts a function into an http.RoundTripper, so the test
// below can fail loudly the instant a build makes a real network request.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// TestBuildProducesABootableImageFromFakeArtifacts is the acceptance test
// for gosd-3zrc: a full `gosd build` for pi-zero-2w, using --artifacts-dir
// to supply fake kernel/firmware files instead of fetching real ones,
// produces a structurally valid image containing the kernel, the rendered
// board templates, and an initramfs with /init, /app, firmware, and
// config.json - all without touching the network.
func TestBuildProducesABootableImageFromFakeArtifacts(t *testing.T) {
	origTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		t.Errorf("unexpected network request to %s during a --artifacts-dir build", r.URL)
		return nil, errors.New("network access is disabled in this test")
	})
	t.Cleanup(func() { http.DefaultTransport = origTransport })

	imgPath := filepath.Join(t.TempDir(), "hello-pi-zero-2w.img")

	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"build", "../../examples/hello",
		"--board", "pi-zero-2w",
		"--artifacts-dir", "testdata/fake-artifacts",
		"-o", imgPath,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("gosd build failed: %v", err)
	}

	d, err := diskfs.Open(imgPath, diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		t.Fatalf("reopening the built image failed: %v", err)
	}
	defer func() { _ = d.Close() }()

	part, err := d.GetPartition(1)
	if err != nil {
		t.Fatalf("GetPartition(1) failed: %v", err)
	}
	if got, want := part.GetStart(), int64(16*1024*1024); got != want {
		t.Errorf("partition 1 starts at byte %d, want %d (16MiB)", got, want)
	}

	fs, err := d.GetFilesystem(1)
	if err != nil {
		t.Fatalf("GetFilesystem(1) failed: %v", err)
	}

	for _, want := range []string{
		"kernel8.img", "bcm2710-rpi-zero-2-w.dtb", "bootcode.bin", "start.elf", "fixup.dat",
		"config.txt", "cmdline.txt", "initramfs.cpio.zst",
	} {
		if _, err := fs.ReadFile(want); err != nil {
			t.Errorf("boot partition is missing %q: %v", want, err)
		}
	}

	cmdlineTxt, err := fs.ReadFile("cmdline.txt")
	if err != nil {
		t.Fatalf("reading cmdline.txt: %v", err)
	}
	if !strings.Contains(string(cmdlineTxt), "gosd.board=pi-zero-2w") {
		t.Errorf("cmdline.txt = %q, want it to contain gosd.board=pi-zero-2w", cmdlineTxt)
	}

	configTxt, err := fs.ReadFile("config.txt")
	if err != nil {
		t.Fatalf("reading config.txt: %v", err)
	}
	if !strings.Contains(string(configTxt), "initramfs initramfs.cpio.zst followkernel") {
		t.Errorf("config.txt = %q, want it to reference initramfs.cpio.zst", configTxt)
	}
	if !strings.Contains(string(configTxt), "dtparam=spi=on") {
		t.Errorf("config.txt = %q, want it to contain dtparam=spi=on (SPI is enabled by default, bean gosd-fnza)", configTxt)
	}

	// Without --usb-gadget the dwc2 overlay must stay off the boot
	// partition, mirroring config.txt's absent dtoverlay line (bean
	// gosd-spjt).
	if _, err := fs.ReadFile("overlays/dwc2.dtbo"); err == nil {
		t.Error("boot partition unexpectedly contains overlays/dwc2.dtbo; it must ship only with --usb-gadget")
	}

	initramfsBytes, err := fs.ReadFile("initramfs.cpio.zst")
	if err != nil {
		t.Fatalf("reading initramfs.cpio.zst: %v", err)
	}
	records := decodeInitramfs(t, initramfsBytes)

	wantEntries := []string{
		"init",
		"app",
		"etc/gosd/config.json",
		"lib/firmware/brcm/brcmfmac43436-sdio.bin",
		"lib/firmware/brcm/brcmfmac43436-sdio.raspberrypi,model-zero-2-w.bin",
		"lib/firmware/brcm/brcmfmac43430b0-sdio.raspberrypi,model-zero-2-w.bin",
		"lib/firmware/brcm/brcmfmac43430-sdio.raspberrypi,model-zero-2-w.bin",
	}
	for _, want := range wantEntries {
		if !hasRecord(records, want) {
			t.Errorf("initramfs is missing entry %q; got entries %v", want, recordNames(records))
		}
	}

	configJSON := recordContent(t, records, "etc/gosd/config.json")
	// The hostname is the app's own name: it's the per-field fallback the
	// device uses whenever the card's config/hostname is left unset.
	for _, want := range []string{`"board":"pi-zero-2w"`, `"hostname":"hello"`, `"configDigests"`} {
		if !strings.Contains(string(configJSON), want) {
			t.Errorf("config.json = %q, want it to contain %q", configJSON, want)
		}
	}

	assertCACertsBaked(t, records)

	// With no --data-size flag, the default (0, no data partition) must
	// produce the single-partition layout. The MBR always has 4 entry slots;
	// an unused slot reads back as a zero-sized partition rather than an
	// error.
	if part2, err := d.GetPartition(2); err == nil && part2.GetSize() != 0 {
		t.Errorf("partition 2 has size %d with no --data-size flag, want no partition 2 (opt-in default)", part2.GetSize())
	}
}

// TestBuildRemovesItsTempDirectory pins bean gosd-7llw: gosd build abandoned
// the working directory it cross-compiles into, leaking one per invocation -
// including one per test in this file - until $TMPDIR held tens of GB of stale
// binaries.
func TestBuildRemovesItsTempDirectory(t *testing.T) {
	disableNetwork(t)

	// os.TempDir reads $TMPDIR on every call, so this redirects the build's
	// own os.MkdirTemp("", "gosd-build-") somewhere this test can inspect.
	tempRoot := t.TempDir()
	t.Setenv("TMPDIR", tempRoot)

	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"build", "../../examples/hello",
		"--board", "pi-zero-2w",
		"--artifacts-dir", "testdata/fake-artifacts",
		"-o", filepath.Join(tempRoot, "hello-pi-zero-2w.img"),
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("gosd build failed: %v", err)
	}

	leaked, err := filepath.Glob(filepath.Join(tempRoot, "gosd-build-*"))
	if err != nil {
		t.Fatalf("globbing %s failed: %v", tempRoot, err)
	}
	if len(leaked) > 0 {
		t.Errorf("gosd build left its working directory behind: %v", leaked)
	}
}

// TestBuildWithDataSizeZeroOmitsTheDataPartition covers the explicit opt-out,
// which is also now the default: --data-size=0 must produce the
// single-partition layout.
func TestBuildWithDataSizeZeroOmitsTheDataPartition(t *testing.T) {
	imgPath := filepath.Join(t.TempDir(), "hello-pi-zero-2w.img")

	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"build", "../../examples/hello",
		"--board", "pi-zero-2w",
		"--artifacts-dir", "testdata/fake-artifacts",
		"--data-size", "0",
		"-o", imgPath,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("gosd build failed: %v", err)
	}

	d, err := diskfs.Open(imgPath, diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		t.Fatalf("reopening the built image failed: %v", err)
	}
	defer func() { _ = d.Close() }()

	// The MBR always has 4 entry slots; an unused slot reads back as a
	// zero-sized partition rather than an error.
	if part2, err := d.GetPartition(2); err == nil && part2.GetSize() != 0 {
		t.Errorf("partition 2 has size %d with --data-size=0, want no partition 2", part2.GetSize())
	}
}

// TestBuildWithDataSizeExpandShipsNoPartitionButFlagsConfig covers
// --data-size=expand: the image itself keeps the single-partition layout
// (staying small to flash), and config.json carries the dataExpand flag that
// makes gosd-init create the partition on first boot.
func TestBuildWithDataSizeExpandShipsNoPartitionButFlagsConfig(t *testing.T) {
	imgPath := filepath.Join(t.TempDir(), "hello-pi-zero-2w.img")

	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"build", "../../examples/hello",
		"--board", "pi-zero-2w",
		"--artifacts-dir", "testdata/fake-artifacts",
		"--data-size", "expand",
		"-o", imgPath,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("gosd build failed: %v", err)
	}

	d, err := diskfs.Open(imgPath, diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		t.Fatalf("reopening the built image failed: %v", err)
	}
	defer func() { _ = d.Close() }()

	if part2, err := d.GetPartition(2); err == nil && part2.GetSize() != 0 {
		t.Errorf("partition 2 has size %d with --data-size=expand, want none in the image (it's created on-device)", part2.GetSize())
	}

	fs, err := d.GetFilesystem(1)
	if err != nil {
		t.Fatalf("GetFilesystem(1) failed: %v", err)
	}
	initramfsBytes, err := fs.ReadFile("initramfs.cpio.zst")
	if err != nil {
		t.Fatalf("reading initramfs.cpio.zst: %v", err)
	}
	configJSON := recordContent(t, decodeInitramfs(t, initramfsBytes), "etc/gosd/config.json")
	if !strings.Contains(string(configJSON), `"dataExpand":true`) {
		t.Errorf("config.json = %q, want it to contain %q", configJSON, `"dataExpand":true`)
	}
}

// TestBuildWithExplicitDataSizeAddsTheDataPartition covers the opt-in path:
// --data-size must produce a second FAT32 data partition sized as
// requested, starting immediately after the boot partition.
func TestBuildWithExplicitDataSizeAddsTheDataPartition(t *testing.T) {
	imgPath := filepath.Join(t.TempDir(), "hello-pi-zero-2w.img")

	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"build", "../../examples/hello",
		"--board", "pi-zero-2w",
		"--artifacts-dir", "testdata/fake-artifacts",
		"--data-size", "512MiB",
		"-o", imgPath,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("gosd build failed: %v", err)
	}

	d, err := diskfs.Open(imgPath, diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		t.Fatalf("reopening the built image failed: %v", err)
	}
	defer func() { _ = d.Close() }()

	dataPart, err := d.GetPartition(2)
	if err != nil {
		t.Fatalf("GetPartition(2) failed: %v", err)
	}
	if got, want := dataPart.GetStart(), int64(272*1024*1024); got != want {
		t.Errorf("partition 2 starts at byte %d, want %d (immediately after the boot partition)", got, want)
	}
	if got, want := dataPart.GetSize(), int64(512*1024*1024); got != want {
		t.Errorf("partition 2 size = %d bytes, want %d (the requested 512MiB)", got, want)
	}
	assertMBRPartitionType(t, d, 2, mbr.Fat32LBA)

	dataFS, err := d.GetFilesystem(2)
	if err != nil {
		t.Fatalf("GetFilesystem(2) failed: %v", err)
	}
	if label := strings.TrimSpace(dataFS.Label()); label != helloDataLabel {
		t.Errorf("data partition label = %q, want %q", label, helloDataLabel)
	}
}

// TestBuildStampsPerAppVolumeLabels is bean gosd-lo7k's acceptance test: the
// two partitions are labelled after the app rather than after GoSD, so a
// flashed card appears on a person's desktop named for what it does — and
// the data label is baked into config.json too, since that is the only thing
// gosd-init has to compare a survivor against on a later boot.
func TestBuildStampsPerAppVolumeLabels(t *testing.T) {
	cases := map[string]struct {
		extraArgs          []string
		wantBoot, wantData string
	}{
		"the app's own name by default": {wantBoot: helloBootLabel, wantData: helloDataLabel},
		"an explicit --label-prefix": {
			extraArgs: []string{"--label-prefix", "web"},
			wantBoot:  "web-boot",
			wantData:  "web-data",
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			imgPath := filepath.Join(t.TempDir(), "hello-pi-zero-2w.img")

			args := append([]string{
				"build", "../../examples/hello",
				"--board", "pi-zero-2w",
				"--artifacts-dir", "testdata/fake-artifacts",
				"--data-size", "512MiB",
				"-o", imgPath,
			}, c.extraArgs...)
			cmd := newRootCmd()
			cmd.SetArgs(args)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("gosd build failed: %v", err)
			}

			d, err := diskfs.Open(imgPath, diskfs.WithOpenMode(diskfs.ReadOnly))
			if err != nil {
				t.Fatalf("reopening the built image failed: %v", err)
			}
			defer func() { _ = d.Close() }()

			bootFS, err := d.GetFilesystem(1)
			if err != nil {
				t.Fatalf("GetFilesystem(1) failed: %v", err)
			}
			if label := strings.TrimSpace(bootFS.Label()); label != c.wantBoot {
				t.Errorf("boot partition label = %q, want %q", label, c.wantBoot)
			}

			dataFS, err := d.GetFilesystem(2)
			if err != nil {
				t.Fatalf("GetFilesystem(2) failed: %v", err)
			}
			if label := strings.TrimSpace(dataFS.Label()); label != c.wantData {
				t.Errorf("data partition label = %q, want %q", label, c.wantData)
			}

			initramfsBytes, err := bootFS.ReadFile("initramfs.cpio.zst")
			if err != nil {
				t.Fatalf("reading initramfs.cpio.zst: %v", err)
			}
			configJSON := recordContent(t, decodeInitramfs(t, initramfsBytes), "etc/gosd/config.json")
			var cfg initcfg.Config
			if err := json.Unmarshal(configJSON, &cfg); err != nil {
				t.Fatalf("config.json = %s is not valid JSON: %v", configJSON, err)
			}
			if cfg.DataLabel != c.wantData {
				t.Errorf("config.json's dataLabel = %q, want %q (it must match the label actually written)", cfg.DataLabel, c.wantData)
			}
		})
	}
}

// TestBuildRefusesADataSizeFAT32CannotHold is the acceptance test for bean
// gosd-mt53: a --data-size past the FAT32 formatter's ceiling has to be
// refused before the build starts, not turned into an image whose data
// partition is silently corrupt (bean gosd-8kdm).
func TestBuildRefusesADataSizeFAT32CannotHold(t *testing.T) {
	imgPath := filepath.Join(t.TempDir(), "hello-pi-zero-2w.img")

	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"build", "../../examples/hello",
		"--board", "pi-zero-2w",
		"--artifacts-dir", "testdata/fake-artifacts",
		"--data-size", "400GiB",
		"-o", imgPath,
	})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("gosd build --data-size=400GiB succeeded, want a refusal")
	}
	for _, want := range []string{diskfmt.GibibytesString(diskfmt.MaxFAT32Bytes()), dataSizeLimitDocsURL} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal %q does not mention %q", err, want)
		}
	}
	if _, statErr := os.Stat(imgPath); !os.IsNotExist(statErr) {
		t.Errorf("gosd build wrote %s despite refusing the data size; the refusal must come first", imgPath)
	}
}

// TestBuildWithBootSizeResizesTheBootPartitionAndShiftsTheDataPartition is
// the acceptance test for bean gosd-m70t: a non-default --boot-size must
// resize the boot partition (partition 1) and shift the data partition to start
// immediately after it, and the build must print a boot-volume usage summary
// naming the board.
func TestBuildWithBootSizeResizesTheBootPartitionAndShiftsTheDataPartition(t *testing.T) {
	origTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		t.Errorf("unexpected network request to %s during a --artifacts-dir build", r.URL)
		return nil, errors.New("network access is disabled in this test")
	})
	t.Cleanup(func() { http.DefaultTransport = origTransport })

	imgPath := filepath.Join(t.TempDir(), "hello-pi-zero-2w.img")
	// 128MiB and 32MiB are both outside the FAT32 self-consistency trim's
	// affected bands (unlike e.g. 64MiB, which the formatter trims by one
	// sector - see internal/diskfmt.LargestSelfConsistentFAT32Bytes), so the
	// image's partitions land at exactly these requested sizes.
	const (
		bootSizeBytes = 128 * 1024 * 1024
		dataSizeBytes = 32 * 1024 * 1024
	)

	cmd := newRootCmd()
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{
		"build", "../../examples/hello",
		"--board", "pi-zero-2w",
		"--artifacts-dir", "testdata/fake-artifacts",
		"--boot-size", "128MiB",
		"--data-size", "32MiB",
		"-o", imgPath,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("gosd build failed: %v", err)
	}

	d, err := diskfs.Open(imgPath, diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		t.Fatalf("reopening the built image failed: %v", err)
	}
	defer func() { _ = d.Close() }()

	bootPart, err := d.GetPartition(1)
	if err != nil {
		t.Fatalf("GetPartition(1) failed: %v", err)
	}
	if got, want := bootPart.GetStart(), int64(16*1024*1024); got != want {
		t.Errorf("partition 1 starts at byte %d, want %d (16MiB, unaffected by --boot-size)", got, want)
	}
	if got, want := bootPart.GetSize(), int64(bootSizeBytes); got != want {
		t.Errorf("partition 1 size = %d bytes, want %d (the requested --boot-size)", got, want)
	}

	wantDataOffset := int64(16*1024*1024 + bootSizeBytes)
	dataPart, err := d.GetPartition(2)
	if err != nil {
		t.Fatalf("GetPartition(2) failed: %v", err)
	}
	if got := dataPart.GetStart(); got != wantDataOffset {
		t.Errorf("partition 2 starts at byte %d, want %d (immediately after the resized boot partition)", got, wantDataOffset)
	}
	if got, want := dataPart.GetSize(), int64(dataSizeBytes); got != want {
		t.Errorf("partition 2 size = %d bytes, want %d (the requested --data-size)", got, want)
	}

	if !strings.Contains(stderr.String(), "pi-zero-2w boot volume:") {
		t.Errorf("stderr = %q, want a boot-volume usage summary naming pi-zero-2w", stderr.String())
	}
}

// TestBuildWithBootSizeAndDataSizeExpandComposeCorrectly is the seam test the
// bean flagged: with gosd-lirl's dataexpand now deriving the data partition's offset
// from the flashed MBR (partition 1's start + size) instead of a mirrored
// 272MiB constant, a non-default --boot-size must produce an image whose MBR
// partition 1 alone already tells gosd-init exactly where to grow the data partition
// on first boot - the image itself still ships no partition 2 (that's the
// point of --data-size=expand), so this only has the MBR to check.
func TestBuildWithBootSizeAndDataSizeExpandComposeCorrectly(t *testing.T) {
	origTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		t.Errorf("unexpected network request to %s during a --artifacts-dir build", r.URL)
		return nil, errors.New("network access is disabled in this test")
	})
	t.Cleanup(func() { http.DefaultTransport = origTransport })

	imgPath := filepath.Join(t.TempDir(), "hello-pi-zero-2w.img")
	const bootSizeBytes = 128 * 1024 * 1024

	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"build", "../../examples/hello",
		"--board", "pi-zero-2w",
		"--artifacts-dir", "testdata/fake-artifacts",
		"--boot-size", "128MiB",
		"--data-size", "expand",
		"-o", imgPath,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("gosd build failed: %v", err)
	}

	d, err := diskfs.Open(imgPath, diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		t.Fatalf("reopening the built image failed: %v", err)
	}
	defer func() { _ = d.Close() }()

	bootPart, err := d.GetPartition(1)
	if err != nil {
		t.Fatalf("GetPartition(1) failed: %v", err)
	}
	if got, want := bootPart.GetStart(), int64(16*1024*1024); got != want {
		t.Errorf("partition 1 starts at byte %d, want %d (16MiB)", got, want)
	}
	if got, want := bootPart.GetSize(), int64(bootSizeBytes); got != want {
		t.Errorf("partition 1 size = %d bytes, want %d (the requested --boot-size); this is exactly what dataexpand reads back to derive the data partition's offset", got, want)
	}

	// --data-size=expand must still keep the image itself single-partition,
	// no matter the boot size: partition 2 is created on the device, not
	// baked into the image.
	if part2, err := d.GetPartition(2); err == nil && part2.GetSize() != 0 {
		t.Errorf("partition 2 has size %d with --data-size=expand, want none in the image (it's created on-device)", part2.GetSize())
	}

	fs, err := d.GetFilesystem(1)
	if err != nil {
		t.Fatalf("GetFilesystem(1) failed: %v", err)
	}
	initramfsBytes, err := fs.ReadFile("initramfs.cpio.zst")
	if err != nil {
		t.Fatalf("reading initramfs.cpio.zst: %v", err)
	}
	configJSON := recordContent(t, decodeInitramfs(t, initramfsBytes), "etc/gosd/config.json")
	if !strings.Contains(string(configJSON), `"dataExpand":true`) {
		t.Errorf("config.json = %q, want it to contain %q", configJSON, `"dataExpand":true`)
	}
}

// TestBuildRefusesABootSizeTooSmallForThePayload is the acceptance test for
// gosd-m70t's fit reporting: a --boot-size too small for what a real build
// actually writes must fail with an actionable error naming --boot-size,
// not go-diskfs's bare "no space left on device" (the disk-full error this
// used to surface as, with no clue which flag to change).
func TestBuildRefusesABootSizeTooSmallForThePayload(t *testing.T) {
	origTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		t.Errorf("unexpected network request to %s during a --artifacts-dir build", r.URL)
		return nil, errors.New("network access is disabled in this test")
	})
	t.Cleanup(func() { http.DefaultTransport = origTransport })

	imgPath := filepath.Join(t.TempDir(), "hello-pi-zero-2w.img")

	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"build", "../../examples/hello",
		"--board", "pi-zero-2w",
		"--artifacts-dir", "testdata/fake-artifacts",
		// The smallest --boot-size the flag parser allows; the real
		// cross-compiled hello binary, gosd-init, and initramfs alone
		// can't fit in 1MiB.
		"--boot-size", "1MiB",
		"-o", imgPath,
	})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("gosd build --boot-size=1MiB succeeded, want a refusal")
	}
	if !errors.Is(err, image.ErrBootPartitionFull) {
		t.Fatalf("gosd build --boot-size=1MiB error = %v, want it to wrap image.ErrBootPartitionFull", err)
	}
	if !strings.Contains(err.Error(), "--boot-size") {
		t.Errorf("refusal %q does not mention --boot-size", err)
	}
}

// assertMBRPartitionType fails the test unless the MBR entry at index has
// the given partition type.
func assertMBRPartitionType(t *testing.T, d *disk.Disk, index int, want mbr.Type) {
	t.Helper()

	table, err := d.GetPartitionTable()
	if err != nil {
		t.Fatalf("GetPartitionTable() failed: %v", err)
	}
	mbrTable, ok := table.(*mbr.Table)
	if !ok {
		t.Fatalf("GetPartitionTable() returned %T, want *mbr.Table", table)
	}
	for _, p := range mbrTable.Partitions {
		if p.Index == index {
			if p.Type != want {
				t.Errorf("partition %d type = %#x, want %#x", index, byte(p.Type), byte(want))
			}
			return
		}
	}
	t.Fatalf("mbr table has no entry for partition %d", index)
}

// TestBuildProducesABootableImageForRadxaZero3EFromFakeArtifacts is the
// acceptance test for gosd-gbsz: a full `gosd build` for radxa-zero-3e,
// using --artifacts-dir to supply fake bootloader/kernel files, produces an
// image with idbloader.img and u-boot.itb raw-written at their locked
// offsets ahead of the boot partition, and a boot partition containing the
// kernel, DTB, initramfs, and a rendered extlinux.conf.
func TestBuildProducesABootableImageForRadxaZero3EFromFakeArtifacts(t *testing.T) {
	origTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		t.Errorf("unexpected network request to %s during a --artifacts-dir build", r.URL)
		return nil, errors.New("network access is disabled in this test")
	})
	t.Cleanup(func() { http.DefaultTransport = origTransport })

	imgPath := filepath.Join(t.TempDir(), "hello-radxa-zero-3e.img")

	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"build", "../../examples/hello",
		"--board", "radxa-zero-3e",
		"--artifacts-dir", "testdata/fake-artifacts",
		"-o", imgPath,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("gosd build failed: %v", err)
	}

	assertRawWriteAt(t, imgPath, 32768, "fake idbloader.img")
	assertRawWriteAt(t, imgPath, 8388608, "fake u-boot.itb")

	d, err := diskfs.Open(imgPath, diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		t.Fatalf("reopening the built image failed: %v", err)
	}
	defer func() { _ = d.Close() }()

	fs, err := d.GetFilesystem(1)
	if err != nil {
		t.Fatalf("GetFilesystem(1) failed: %v", err)
	}

	for _, want := range []string{"Image", "rk3566-radxa-zero-3e.dtb", "initramfs.cpio.zst", "extlinux/extlinux.conf"} {
		if _, err := fs.ReadFile(want); err != nil {
			t.Errorf("boot partition is missing %q: %v", want, err)
		}
	}

	extlinuxConf, err := fs.ReadFile("extlinux/extlinux.conf")
	if err != nil {
		t.Fatalf("reading extlinux/extlinux.conf: %v", err)
	}
	wantExtlinuxConf := "default gosd\n" +
		"timeout 0\n" +
		"label gosd\n" +
		"    kernel /Image\n" +
		"    fdt /rk3566-radxa-zero-3e.dtb\n" +
		"    initrd /initramfs.cpio.zst\n" +
		"    append console=ttyS2,1500000n8 quiet init=/init gosd.board=radxa-zero-3e panic=10\n"
	if string(extlinuxConf) != wantExtlinuxConf {
		t.Errorf("extlinux.conf = %q, want %q", extlinuxConf, wantExtlinuxConf)
	}

	initramfsBytes, err := fs.ReadFile("initramfs.cpio.zst")
	if err != nil {
		t.Fatalf("reading initramfs.cpio.zst: %v", err)
	}
	assertCACertsBaked(t, decodeInitramfs(t, initramfsBytes))
}

// TestBuildConsoleBaudOverridesRockchipExtlinuxConf is the acceptance test
// for gosd-zp9s's --console-baud flag on the Rockchip boot chain: it
// overrides extlinux.conf's console= rate while leaving the UART device
// (ttyS2) and everything else in the file unchanged.
func TestBuildConsoleBaudOverridesRockchipExtlinuxConf(t *testing.T) {
	imgPath := filepath.Join(t.TempDir(), "hello-radxa-zero-3e.img")

	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"build", "../../examples/hello",
		"--board", "radxa-zero-3e",
		"--artifacts-dir", "testdata/fake-artifacts",
		"--console-baud", "115200",
		"-o", imgPath,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("gosd build failed: %v", err)
	}

	d, err := diskfs.Open(imgPath, diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		t.Fatalf("reopening the built image failed: %v", err)
	}
	defer func() { _ = d.Close() }()

	fs, err := d.GetFilesystem(1)
	if err != nil {
		t.Fatalf("GetFilesystem(1) failed: %v", err)
	}

	extlinuxConf, err := fs.ReadFile("extlinux/extlinux.conf")
	if err != nil {
		t.Fatalf("reading extlinux/extlinux.conf: %v", err)
	}
	if !strings.Contains(string(extlinuxConf), "console=ttyS2,115200n8") {
		t.Errorf("extlinux.conf = %q, want it to contain console=ttyS2,115200n8", extlinuxConf)
	}
	if strings.Contains(string(extlinuxConf), "1500000") {
		t.Errorf("extlinux.conf = %q, want the default 1500000 rate gone once overridden", extlinuxConf)
	}
}

// TestBuildConsoleBaudOverridesPiCmdlineTxt is the acceptance test for
// gosd-zp9s's --console-baud flag on the Pi boot chain: it overrides
// cmdline.txt's console= rate the same way.
func TestBuildConsoleBaudOverridesPiCmdlineTxt(t *testing.T) {
	imgPath := filepath.Join(t.TempDir(), "hello-pi-zero-2w.img")

	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"build", "../../examples/hello",
		"--board", "pi-zero-2w",
		"--artifacts-dir", "testdata/fake-artifacts",
		"--console-baud", "57600",
		"-o", imgPath,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("gosd build failed: %v", err)
	}

	d, err := diskfs.Open(imgPath, diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		t.Fatalf("reopening the built image failed: %v", err)
	}
	defer func() { _ = d.Close() }()

	fs, err := d.GetFilesystem(1)
	if err != nil {
		t.Fatalf("GetFilesystem(1) failed: %v", err)
	}

	cmdlineTxt, err := fs.ReadFile("cmdline.txt")
	if err != nil {
		t.Fatalf("reading cmdline.txt: %v", err)
	}
	if !strings.Contains(string(cmdlineTxt), "console=serial0,57600") {
		t.Errorf("cmdline.txt = %q, want it to contain console=serial0,57600", cmdlineTxt)
	}
	if strings.Contains(string(cmdlineTxt), "115200") {
		t.Errorf("cmdline.txt = %q, want the default 115200 rate gone once overridden", cmdlineTxt)
	}
}

// TestBuildConsoleBaudFailsActionablyForIncapableBoard confirms --console-
// baud refuses to build for qemu-virt (whose console has no baud rate at
// all - see qemuvirt.ConsoleBaudSupport) rather than silently ignoring the
// flag.
func TestBuildConsoleBaudFailsActionablyForIncapableBoard(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"build", "../../examples/hello",
		"--board", "qemu-virt",
		"--artifacts-dir", "testdata/fake-artifacts",
		"--console-baud", "115200",
		"-o", filepath.Join(t.TempDir(), "hello-qemu-virt.img"),
	})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("gosd build --board=qemu-virt --console-baud=115200 succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "qemu-virt") {
		t.Errorf("error = %q, want it to name qemu-virt", err.Error())
	}
}

// TestBuildUsbGadgetShipsDwc2OverlayForPiZeros is the acceptance test for
// bean gosd-spjt: a `gosd build --usb-gadget` for each Pi Zero board must
// put the pinned dwc2 overlay at overlays/dwc2.dtbo on the FAT boot
// partition, alongside config.txt's dtoverlay line — before this, the line
// was rendered but the .dtbo never shipped, and the Pi firmware skipped the
// overlay silently, leaving the app with no UDC. The fake artifact stands in
// for the raspberrypi/firmware download exactly like bootcode.bin's; the
// network tripwire proves --artifacts-dir keeps satisfying every pinned
// fetch.
func TestBuildUsbGadgetShipsDwc2OverlayForPiZeros(t *testing.T) {
	origTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		t.Errorf("unexpected network request to %s during a --artifacts-dir build", r.URL)
		return nil, errors.New("network access is disabled in this test")
	})
	t.Cleanup(func() { http.DefaultTransport = origTransport })

	for _, board := range []string{"pi-zero-2w", "pi-zero-w"} {
		t.Run(board, func(t *testing.T) {
			imgPath := filepath.Join(t.TempDir(), "hello-"+board+".img")

			cmd := newRootCmd()
			cmd.SetArgs([]string{
				"build", "../../examples/hello",
				"--board", board,
				"--artifacts-dir", "testdata/fake-artifacts",
				"--usb-gadget",
				"-o", imgPath,
			})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("gosd build --board=%s --usb-gadget failed: %v", board, err)
			}

			d, err := diskfs.Open(imgPath, diskfs.WithOpenMode(diskfs.ReadOnly))
			if err != nil {
				t.Fatalf("reopening the built image failed: %v", err)
			}
			defer func() { _ = d.Close() }()

			fs, err := d.GetFilesystem(1)
			if err != nil {
				t.Fatalf("GetFilesystem(1) failed: %v", err)
			}

			dtbo, err := fs.ReadFile("overlays/dwc2.dtbo")
			if err != nil {
				t.Fatalf("boot partition is missing overlays/dwc2.dtbo: %v", err)
			}
			if got, want := string(dtbo), "fake dwc2.dtbo content for gosd integration tests\n"; got != want {
				t.Errorf("overlays/dwc2.dtbo content = %q, want the resolved artifact's content %q", got, want)
			}

			configTxt, err := fs.ReadFile("config.txt")
			if err != nil {
				t.Fatalf("reading config.txt: %v", err)
			}
			if !strings.Contains(string(configTxt), "dtoverlay=dwc2,dr_mode=peripheral") {
				t.Errorf("config.txt = %q, want the dwc2 peripheral-mode overlay line with --usb-gadget", configTxt)
			}
		})
	}
}

// TestBuildProducesABootableImageForNanopiZero2FromFakeArtifacts is the
// acceptance test for gosd-wskc: an explicit `gosd build --board=nanopi-
// zero2`, using --artifacts-dir to supply fake bootloader/kernel files,
// produces an image with idbloader.img and u-boot.itb raw-written at their
// locked offsets ahead of the boot partition, and a boot partition
// containing the kernel, DTB, initramfs, and a rendered extlinux.conf - the
// same shape as the Radxa Zero 3E. nanopi-zero2 is now a public board, so
// it's also part of the default build set (TestBuildWithNoBoardFlagBuilds
// AllBoards) and appears in catalog output (TestBuildCatalogForNanopiZero2
// WritesEntry below).
func TestBuildProducesABootableImageForNanopiZero2FromFakeArtifacts(t *testing.T) {
	origTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		t.Errorf("unexpected network request to %s during a --artifacts-dir build", r.URL)
		return nil, errors.New("network access is disabled in this test")
	})
	t.Cleanup(func() { http.DefaultTransport = origTransport })

	imgPath := filepath.Join(t.TempDir(), "hello-nanopi-zero2.img")

	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"build", "../../examples/hello",
		"--board", "nanopi-zero2",
		"--artifacts-dir", "testdata/fake-artifacts",
		"-o", imgPath,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("gosd build --board=nanopi-zero2 failed: %v", err)
	}

	assertRawWriteAt(t, imgPath, 32768, "fake idbloader.img")
	assertRawWriteAt(t, imgPath, 8388608, "fake u-boot.itb")

	d, err := diskfs.Open(imgPath, diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		t.Fatalf("reopening the built image failed: %v", err)
	}
	defer func() { _ = d.Close() }()

	fs, err := d.GetFilesystem(1)
	if err != nil {
		t.Fatalf("GetFilesystem(1) failed: %v", err)
	}

	for _, want := range []string{"Image", "rk3528-nanopi-zero2.dtb", "initramfs.cpio.zst", "extlinux/extlinux.conf"} {
		if _, err := fs.ReadFile(want); err != nil {
			t.Errorf("boot partition is missing %q: %v", want, err)
		}
	}

	extlinuxConf, err := fs.ReadFile("extlinux/extlinux.conf")
	if err != nil {
		t.Fatalf("reading extlinux/extlinux.conf: %v", err)
	}
	wantExtlinuxConf := "default gosd\n" +
		"timeout 0\n" +
		"label gosd\n" +
		"    kernel /Image\n" +
		"    fdt /rk3528-nanopi-zero2.dtb\n" +
		"    initrd /initramfs.cpio.zst\n" +
		"    append console=ttyS0,1500000n8 quiet init=/init gosd.board=nanopi-zero2 panic=10\n"
	if string(extlinuxConf) != wantExtlinuxConf {
		t.Errorf("extlinux.conf = %q, want %q", extlinuxConf, wantExtlinuxConf)
	}

	initramfsBytes, err := fs.ReadFile("initramfs.cpio.zst")
	if err != nil {
		t.Fatalf("reading initramfs.cpio.zst: %v", err)
	}
	assertCACertsBaked(t, decodeInitramfs(t, initramfsBytes))
}

// TestBuildProducesABootableImageForPiZeroWFromFakeArtifacts is the
// acceptance test for gosd-et0q: a full `gosd build` for pi-zero-w, using
// --artifacts-dir to supply fake kernel/firmware files instead of fetching
// real ones, produces a structurally valid 32-bit image. Unlike the other
// boards' fake-artifacts tests, /app and /init here are NOT fakes — the
// pipeline really cross-compiles examples/hello and gosd-init for
// GOARCH=arm GOARM=6 (this board's Arch()), so this test closes the loop on
// the multi-arch build work (gosd-2j6z) by asserting the initramfs actually
// contains 32-bit ARM ELF binaries, not just that a build "succeeded".
func TestBuildProducesABootableImageForPiZeroWFromFakeArtifacts(t *testing.T) {
	origTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		t.Errorf("unexpected network request to %s during a --artifacts-dir build", r.URL)
		return nil, errors.New("network access is disabled in this test")
	})
	t.Cleanup(func() { http.DefaultTransport = origTransport })

	imgPath := filepath.Join(t.TempDir(), "hello-pi-zero-w.img")

	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"build", "../../examples/hello",
		"--board", "pi-zero-w",
		"--artifacts-dir", "testdata/fake-artifacts",
		"-o", imgPath,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("gosd build --board=pi-zero-w failed: %v", err)
	}

	d, err := diskfs.Open(imgPath, diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		t.Fatalf("reopening the built image failed: %v", err)
	}
	defer func() { _ = d.Close() }()

	fs, err := d.GetFilesystem(1)
	if err != nil {
		t.Fatalf("GetFilesystem(1) failed: %v", err)
	}

	for _, want := range []string{
		"kernel.img", "bcm2835-rpi-zero-w.dtb", "bootcode.bin", "start.elf", "fixup.dat",
		"config.txt", "cmdline.txt", "initramfs.cpio.zst",
	} {
		if _, err := fs.ReadFile(want); err != nil {
			t.Errorf("boot partition is missing %q: %v", want, err)
		}
	}

	cmdlineTxt, err := fs.ReadFile("cmdline.txt")
	if err != nil {
		t.Fatalf("reading cmdline.txt: %v", err)
	}
	if !strings.Contains(string(cmdlineTxt), "gosd.board=pi-zero-w") {
		t.Errorf("cmdline.txt = %q, want it to contain gosd.board=pi-zero-w", cmdlineTxt)
	}

	configTxt, err := fs.ReadFile("config.txt")
	if err != nil {
		t.Fatalf("reading config.txt: %v", err)
	}
	if strings.Contains(string(configTxt), "arm_64bit") {
		t.Errorf("config.txt = %q, want no arm_64bit line (pi-zero-w is 32-bit only)", configTxt)
	}
	if !strings.Contains(string(configTxt), "kernel=kernel.img") {
		t.Errorf("config.txt = %q, want it to reference kernel.img", configTxt)
	}
	if !strings.Contains(string(configTxt), "dtparam=spi=on") {
		t.Errorf("config.txt = %q, want it to contain dtparam=spi=on (SPI is enabled by default, bean gosd-fnza)", configTxt)
	}

	initramfsBytes, err := fs.ReadFile("initramfs.cpio.zst")
	if err != nil {
		t.Fatalf("reading initramfs.cpio.zst: %v", err)
	}
	records := decodeInitramfs(t, initramfsBytes)

	wantEntries := []string{
		"init",
		"app",
		"etc/gosd/config.json",
		"lib/firmware/brcm/cyfmac43430-sdio.bin",
		"lib/firmware/brcm/brcmfmac43430-sdio.raspberrypi,model-zero-w.bin",
		"lib/firmware/brcm/brcmfmac43430-sdio.raspberrypi,model-zero-w.clm_blob",
		"lib/firmware/brcm/brcmfmac43430-sdio.raspberrypi,model-zero-w.txt",
	}
	for _, want := range wantEntries {
		if !hasRecord(records, want) {
			t.Errorf("initramfs is missing entry %q; got entries %v", want, recordNames(records))
		}
	}

	configJSON := recordContent(t, records, "etc/gosd/config.json")
	for _, want := range []string{`"board":"pi-zero-w"`, `"hostname":"hello"`, `"configDigests"`} {
		if !strings.Contains(string(configJSON), want) {
			t.Errorf("config.json = %q, want it to contain %q", configJSON, want)
		}
	}

	// The real acceptance criterion: /app and /init must be genuine 32-bit
	// ARM ELF binaries, since pi-zero-w's Arch() (GOARCH=arm, GOARM=6)
	// isn't faked — the build pipeline really cross-compiled them.
	for _, name := range []string{"app", "init"} {
		assertELF32Arm(t, records, name)
	}

	assertCACertsBaked(t, records)
}

// assertELF32Arm fails the test unless the cpio record named name parses as
// a 32-bit ARM ELF binary (ELFCLASS32, EM_ARM) — the shape a real GOARCH=arm
// GOARM=6 cross-compile produces.
func assertELF32Arm(t *testing.T, records []cpio.Record, name string) {
	t.Helper()

	rec, ok := findRecord(records, name)
	if !ok {
		t.Fatalf("no record named %q found in initramfs", name)
	}

	f, err := elf.NewFile(rec)
	if err != nil {
		t.Fatalf("%s is not a valid ELF binary: %v", name, err)
	}
	defer func() { _ = f.Close() }()

	if f.Class != elf.ELFCLASS32 {
		t.Errorf("%s: Class = %v, want %v (32-bit)", name, f.Class, elf.ELFCLASS32)
	}
	if f.Machine != elf.EM_ARM {
		t.Errorf("%s: Machine = %v, want %v (arm)", name, f.Machine, elf.EM_ARM)
	}
}

func findRecord(records []cpio.Record, name string) (cpio.Record, bool) {
	for _, r := range records {
		if r.Name == name {
			return r, true
		}
	}
	return cpio.Record{}, false
}

// TestBuildWithNoBoardFlagBuildsAllBoards confirms that omitting --board (as
// gosd's locked "no --board builds every board" decision requires) produces
// an image for every public board, not just a subset - and nothing else, so
// an internal-only board can't leak into the default build set.
func TestBuildWithNoBoardFlagBuildsAllBoards(t *testing.T) {
	origTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		t.Errorf("unexpected network request to %s during a --artifacts-dir build", r.URL)
		return nil, errors.New("network access is disabled in this test")
	})
	t.Cleanup(func() { http.DefaultTransport = origTransport })

	outDir := t.TempDir()

	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"build", "../../examples/hello",
		"--artifacts-dir", "testdata/fake-artifacts",
		"-o", outDir,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("gosd build failed: %v", err)
	}

	public := boards.All()
	for _, b := range public {
		want := "hello-" + b.Name() + ".img"
		path := filepath.Join(outDir, want)
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("expected output image %q was not produced: %v", path, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("output image %q is empty", path)
		}
	}

	// qemu-virt is the only internal-only board (cubie-a5e went public in
	// bean gosd-zh95's activation): the default no---board build must
	// produce exactly the public boards' images, never one for qemu-virt.
	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatalf("reading output directory: %v", err)
	}
	var imgNames []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".img") {
			imgNames = append(imgNames, e.Name())
		}
	}
	if len(imgNames) != len(public) {
		t.Errorf("default build produced %d .img files (%v), want exactly %d (internal-only boards must stay excluded)", len(imgNames), imgNames, len(public))
	}
	if _, err := os.Stat(filepath.Join(outDir, "hello-qemu-virt.img")); err == nil {
		t.Error("default build produced hello-qemu-virt.img; that board is internal-only and must be excluded from the default build set")
	}
}

// TestBuildProducesAQemuVirtImageFromFakeArtifacts is the acceptance test for
// gosd-2v40: an explicit `gosd build --board=qemu-virt`, using
// --artifacts-dir to supply a fake kernel image, produces an image whose
// boot partition contains exactly the kernel (Image), the initramfs, and
// the config tree (added by the pipeline for every board) - no config.txt or
// extlinux.conf, since qemu boots -kernel/-initrd directly (see
// internal/boards/qemuvirt).
func TestBuildProducesAQemuVirtImageFromFakeArtifacts(t *testing.T) {
	origTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		t.Errorf("unexpected network request to %s during a --artifacts-dir build", r.URL)
		return nil, errors.New("network access is disabled in this test")
	})
	t.Cleanup(func() { http.DefaultTransport = origTransport })

	imgPath := filepath.Join(t.TempDir(), "hello-qemu-virt.img")

	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"build", "../../examples/hello",
		"--board", "qemu-virt",
		"--artifacts-dir", "testdata/fake-artifacts",
		"-o", imgPath,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("gosd build --board=qemu-virt failed: %v", err)
	}

	d, err := diskfs.Open(imgPath, diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		t.Fatalf("reopening the built image failed: %v", err)
	}
	defer func() { _ = d.Close() }()

	fs, err := d.GetFilesystem(1)
	if err != nil {
		t.Fatalf("GetFilesystem(1) failed: %v", err)
	}

	for _, want := range []string{"Image", "initramfs.cpio.zst", "config/explain.md", "config/hostname"} {
		if _, err := fs.ReadFile(want); err != nil {
			t.Errorf("boot partition is missing %q: %v", want, err)
		}
	}
	for _, absent := range []string{"config.txt", "cmdline.txt", "extlinux/extlinux.conf"} {
		if _, err := fs.ReadFile(absent); err == nil {
			t.Errorf("boot partition unexpectedly contains %q; qemu-virt has no on-device bootloader to configure", absent)
		}
	}

	initramfsBytes, err := fs.ReadFile("initramfs.cpio.zst")
	if err != nil {
		t.Fatalf("reading initramfs.cpio.zst: %v", err)
	}
	assertCACertsBaked(t, decodeInitramfs(t, initramfsBytes))
}

// extractPartitionRegion copies the byte range [offset, offset+length) of
// imgPath into a fresh temp file, so a partition's raw bytes can be handed to
// diskfmt.Inspect exactly as it would see a real block device - mirrors
// internal/image's own extractRegion test helper (image_test.go), which this
// package can't import directly since it's an internal test helper of
// another package.
func extractPartitionRegion(t *testing.T, imgPath string, offset, length int64) string {
	t.Helper()
	src, err := os.Open(imgPath)
	if err != nil {
		t.Fatalf("opening %s to extract a region: %v", imgPath, err)
	}
	defer func() { _ = src.Close() }()

	regionPath := filepath.Join(t.TempDir(), "region.img")
	dst, err := os.Create(regionPath)
	if err != nil {
		t.Fatalf("creating %s: %v", regionPath, err)
	}
	if _, err := io.Copy(dst, io.NewSectionReader(src, offset, length)); err != nil {
		t.Fatalf("copying region [%d, %d) from %s: %v", offset, offset+length, imgPath, err)
	}
	if err := dst.Close(); err != nil {
		t.Fatalf("closing %s: %v", regionPath, err)
	}
	return regionPath
}

// TestBuildDataFilesystemEXT4ProducesAReadableEXT4DataPartition is bean
// gosd-95yu's build-level acceptance test: `gosd build --board=qemu-virt
// --data-filesystem=ext4` (qemu-virt is one of the boards ext4 is supported
// on - see internal/boards/qemuvirt.EXT4Support) must mark partition 2 as
// MBR type 0x83 (Linux/ext4), format it as a real ext4 filesystem labelled
// data partition that diskfmt.Inspect reads back, and bake config.json's
// dataFilesystem field as "ext4".
func TestBuildDataFilesystemEXT4ProducesAReadableEXT4DataPartition(t *testing.T) {
	origTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		t.Errorf("unexpected network request to %s during a --artifacts-dir build", r.URL)
		return nil, errors.New("network access is disabled in this test")
	})
	t.Cleanup(func() { http.DefaultTransport = origTransport })

	imgPath := filepath.Join(t.TempDir(), "hello-qemu-virt.img")
	dataSizeBytes := diskfmt.EXT4GoldenData.MinBytes()

	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"build", "../../examples/hello",
		"--board", "qemu-virt",
		"--artifacts-dir", "testdata/fake-artifacts",
		"--data-filesystem", "ext4",
		"--data-size", strconv.FormatInt(dataSizeBytes, 10),
		"-o", imgPath,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("gosd build --data-filesystem=ext4 failed: %v", err)
	}

	d, err := diskfs.Open(imgPath, diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		t.Fatalf("reopening the built image failed: %v", err)
	}
	defer func() { _ = d.Close() }()

	assertMBRPartitionType(t, d, 2, mbr.Linux)

	dataPart, err := d.GetPartition(2)
	if err != nil {
		t.Fatalf("GetPartition(2) failed: %v", err)
	}
	if got := dataPart.GetSize(); got != dataSizeBytes {
		t.Errorf("partition 2 size = %d bytes, want %d (the requested --data-size)", got, dataSizeBytes)
	}

	regionPath := extractPartitionRegion(t, imgPath, dataPart.GetStart(), dataPart.GetSize())
	contents, err := diskfmt.Inspect(regionPath)
	if err != nil {
		t.Fatalf("diskfmt.Inspect on the extracted ext4 data partition failed: %v", err)
	}
	if contents.FS != diskfmt.EXT4 {
		t.Errorf("Inspect().FS = %v, want ext4", contents.FS)
	}
	if contents.Label != helloDataLabel {
		t.Errorf("Inspect().Label = %q, want %q", contents.Label, helloDataLabel)
	}

	fs, err := d.GetFilesystem(1)
	if err != nil {
		t.Fatalf("GetFilesystem(1) failed: %v", err)
	}
	initramfsBytes, err := fs.ReadFile("initramfs.cpio.zst")
	if err != nil {
		t.Fatalf("reading initramfs.cpio.zst: %v", err)
	}
	configJSON := recordContent(t, decodeInitramfs(t, initramfsBytes), "etc/gosd/config.json")
	if !strings.Contains(string(configJSON), `"dataFilesystem":"ext4"`) {
		t.Errorf("config.json = %q, want it to contain %q", configJSON, `"dataFilesystem":"ext4"`)
	}
}

// TestBuildDefaultDataFilesystemIsStillFAT32 confirms that omitting
// --data-filesystem entirely still produces a FAT32 (MBR type 0x0C)
// FAT32 data partition and bakes config.json's dataFilesystem as "fat32" -
// bean gosd-95yu's default must never be able to flip to ext4 silently.
func TestBuildDefaultDataFilesystemIsStillFAT32(t *testing.T) {
	imgPath := filepath.Join(t.TempDir(), "hello-pi-zero-2w.img")

	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"build", "../../examples/hello",
		"--board", "pi-zero-2w",
		"--artifacts-dir", "testdata/fake-artifacts",
		"--data-size", "8MiB",
		"-o", imgPath,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("gosd build failed: %v", err)
	}

	d, err := diskfs.Open(imgPath, diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		t.Fatalf("reopening the built image failed: %v", err)
	}
	defer func() { _ = d.Close() }()

	assertMBRPartitionType(t, d, 2, mbr.Fat32LBA)

	fs, err := d.GetFilesystem(1)
	if err != nil {
		t.Fatalf("GetFilesystem(1) failed: %v", err)
	}
	initramfsBytes, err := fs.ReadFile("initramfs.cpio.zst")
	if err != nil {
		t.Fatalf("reading initramfs.cpio.zst: %v", err)
	}
	configJSON := recordContent(t, decodeInitramfs(t, initramfsBytes), "etc/gosd/config.json")
	if !strings.Contains(string(configJSON), `"dataFilesystem":"fat32"`) {
		t.Errorf("config.json = %q, want it to contain %q", configJSON, `"dataFilesystem":"fat32"`)
	}
}

// TestBuildCatalogForQemuVirtOnlyWritesNothing confirms gosd-2v40's chosen
// behavior for --catalog when every selected board is internal-only: no
// os_list.json is written, and the build itself still succeeds (this is not
// treated as an error - see writeCatalog's doc comment for why).
func TestBuildCatalogForQemuVirtOnlyWritesNothing(t *testing.T) {
	origTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		t.Errorf("unexpected network request to %s during a --artifacts-dir build", r.URL)
		return nil, errors.New("network access is disabled in this test")
	})
	t.Cleanup(func() { http.DefaultTransport = origTransport })

	outDir := t.TempDir()
	imgPath := filepath.Join(outDir, "hello-qemu-virt.img")

	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"build", "../../examples/hello",
		"--board", "qemu-virt",
		"--artifacts-dir", "testdata/fake-artifacts",
		"--catalog",
		"--publish-base-url", "https://example.com/downloads",
		"-o", imgPath,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("gosd build --board=qemu-virt --catalog failed: %v", err)
	}

	if _, err := os.Stat(imgPath); err != nil {
		t.Errorf("the image itself should still be built: %v", err)
	}
	for _, listPath := range []string{
		filepath.Join(outDir, "os_list.json"),
		filepath.Join(outDir, "hello-qemu-virt.os_list.json"),
	} {
		if _, err := os.Stat(listPath); err == nil {
			t.Errorf("%s was written for a qemu-virt-only build; qemu-virt is internal-only and must never appear in a catalog", listPath)
		}
	}
}

// TestBuildProducesABootableImageForPi3BFromFakeArtifacts is the acceptance
// test for bean gosd-ypg1: an explicit `gosd build --board=pi-3b`, using
// --artifacts-dir to supply fake kernel/firmware files, produces an image
// whose boot partition carries the GPU-ROM boot flow (kernel8.img, both
// family DTBs - the firmware picks the 3B's or the 3B+'s by board revision
// (bean gosd-oq0z) - boot firmware, config.txt with arm_64bit=1 and no
// dtoverlay, cmdline.txt) and whose initramfs carries the Cypress 43430
// WiFi blobs under their 3-model-b alias names. pi-3b is public since bean
// gosd-7wv9's activation, so it's also covered by the default all-boards
// build (TestBuildWithNoBoardFlagBuildsAllBoards) and by catalog output
// (TestBuildCatalogForPi3BWritesEntry).
func TestBuildProducesABootableImageForPi3BFromFakeArtifacts(t *testing.T) {
	origTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		t.Errorf("unexpected network request to %s during a --artifacts-dir build", r.URL)
		return nil, errors.New("network access is disabled in this test")
	})
	t.Cleanup(func() { http.DefaultTransport = origTransport })

	imgPath := filepath.Join(t.TempDir(), "hello-pi-3b.img")

	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"build", "../../examples/hello",
		"--board", "pi-3b",
		"--artifacts-dir", "testdata/fake-artifacts",
		"-o", imgPath,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("gosd build --board=pi-3b failed: %v", err)
	}

	d, err := diskfs.Open(imgPath, diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		t.Fatalf("reopening the built image failed: %v", err)
	}
	defer func() { _ = d.Close() }()

	fs, err := d.GetFilesystem(1)
	if err != nil {
		t.Fatalf("GetFilesystem(1) failed: %v", err)
	}

	for _, want := range []string{
		"kernel8.img", "bcm2710-rpi-3-b.dtb", "bcm2710-rpi-3-b-plus.dtb",
		"bootcode.bin", "start.elf", "fixup.dat",
		"config.txt", "cmdline.txt", "initramfs.cpio.zst",
	} {
		if _, err := fs.ReadFile(want); err != nil {
			t.Errorf("boot partition is missing %q: %v", want, err)
		}
	}

	cmdlineTxt, err := fs.ReadFile("cmdline.txt")
	if err != nil {
		t.Fatalf("reading cmdline.txt: %v", err)
	}
	if !strings.Contains(string(cmdlineTxt), "gosd.board=pi-3b") {
		t.Errorf("cmdline.txt = %q, want it to contain gosd.board=pi-3b", cmdlineTxt)
	}
	if !strings.Contains(string(cmdlineTxt), "console=serial0,115200") {
		t.Errorf("cmdline.txt = %q, want the default console=serial0,115200 (mini-UART)", cmdlineTxt)
	}

	configTxt, err := fs.ReadFile("config.txt")
	if err != nil {
		t.Fatalf("reading config.txt: %v", err)
	}
	if !strings.Contains(string(configTxt), "arm_64bit=1") {
		t.Errorf("config.txt = %q, want arm_64bit=1 (the 3B boots the arm64 kernel8.img)", configTxt)
	}
	if !strings.Contains(string(configTxt), "kernel=kernel8.img") {
		t.Errorf("config.txt = %q, want it to reference kernel8.img", configTxt)
	}
	if strings.Contains(string(configTxt), "dtoverlay") {
		t.Errorf("config.txt = %q, want no dtoverlay line: the 3B's hub-wired USB can never be a peripheral", configTxt)
	}

	initramfsBytes, err := fs.ReadFile("initramfs.cpio.zst")
	if err != nil {
		t.Fatalf("reading initramfs.cpio.zst: %v", err)
	}
	records := decodeInitramfs(t, initramfsBytes)

	wantEntries := []string{
		"init",
		"app",
		"etc/gosd/config.json",
		"lib/firmware/brcm/cyfmac43430-sdio.bin",
		"lib/firmware/brcm/brcmfmac43430-sdio.raspberrypi,3-model-b.bin",
		"lib/firmware/brcm/brcmfmac43430-sdio.raspberrypi,3-model-b.clm_blob",
		"lib/firmware/brcm/brcmfmac43430-sdio.raspberrypi,3-model-b.txt",
	}
	for _, want := range wantEntries {
		if !hasRecord(records, want) {
			t.Errorf("initramfs is missing entry %q; got entries %v", want, recordNames(records))
		}
	}

	configJSON := recordContent(t, records, "etc/gosd/config.json")
	for _, want := range []string{`"board":"pi-3b"`, `"hostname":"hello"`, `"configDigests"`} {
		if !strings.Contains(string(configJSON), want) {
			t.Errorf("config.json = %q, want it to contain %q", configJSON, want)
		}
	}

	assertCACertsBaked(t, records)
}

// TestBuildUsbGadgetFailsActionablyForPi3B confirms `gosd build
// --board=pi-3b --usb-gadget` fails before any image assembly with an error
// naming the board and the hardware reason (gosd-5pnr's capability check):
// the 3B's SoC USB is hard-wired through its LAN9514 hub, so no UDC can
// ever exist for the gadget package to bind.
func TestBuildUsbGadgetFailsActionablyForPi3B(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"build", "../../examples/hello",
		"--board", "pi-3b",
		"--artifacts-dir", "testdata/fake-artifacts",
		"--usb-gadget",
		"-o", filepath.Join(t.TempDir(), "hello-pi-3b.img"),
	})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("gosd build --board=pi-3b --usb-gadget succeeded, want an error")
	}
	for _, want := range []string{"pi-3b", "LAN9514"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to mention %q", err.Error(), want)
		}
	}
}

// TestBuildCatalogForPi3BWritesEntry confirms that, now that pi-3b is a
// public board (gosd-7wv9's flip), --catalog on a pi-3b-only build writes a
// real os_list.json entry - unlike qemu-virt (still internal-only, see
// TestBuildCatalogForQemuVirtOnlyWritesNothing above) - carrying the
// official "pi3-64bit" Imager device tag (the "Raspberry Pi 3" device's
// arm64 tag, verified live against the official catalog on 2026-07-26; see
// internal/catalog.boardImagerDeviceTags).
func TestBuildCatalogForPi3BWritesEntry(t *testing.T) {
	origTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		t.Errorf("unexpected network request to %s during a --artifacts-dir build", r.URL)
		return nil, errors.New("network access is disabled in this test")
	})
	t.Cleanup(func() { http.DefaultTransport = origTransport })

	outDir := t.TempDir()
	imgPath := filepath.Join(outDir, "hello-pi-3b.img")

	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"build", "../../examples/hello",
		"--board", "pi-3b",
		"--artifacts-dir", "testdata/fake-artifacts",
		"--catalog",
		"--publish-base-url", "https://example.com/downloads",
		"-o", imgPath,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("gosd build --board=pi-3b --catalog failed: %v", err)
	}

	if _, err := os.Stat(imgPath); err != nil {
		t.Errorf("the image itself should be built: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(outDir, "hello-pi-3b.os_list.json"))
	if err != nil {
		t.Fatalf("reading hello-pi-3b.os_list.json: %v", err)
	}

	var list struct {
		OSList []struct {
			Name    string   `json:"name"`
			Devices []string `json:"devices"`
		} `json:"os_list"`
	}
	if err := json.Unmarshal(data, &list); err != nil {
		t.Fatalf("unmarshaling hello-pi-3b.os_list.json: %v", err)
	}
	if len(list.OSList) != 1 {
		t.Fatalf("hello-pi-3b.os_list.json has %d entries, want 1", len(list.OSList))
	}

	entry := list.OSList[0]
	if want := "hello (Raspberry Pi 3B)"; entry.Name != want {
		t.Errorf("name = %q, want %q", entry.Name, want)
	}
	if len(entry.Devices) != 1 || entry.Devices[0] != "pi3-64bit" {
		t.Errorf("devices = %v, want [\"pi3-64bit\"] (the official Raspberry Pi 3 device tag for arm64 images)", entry.Devices)
	}
}

// TestBuildProducesABootableImageForRock4SEFromFakeArtifacts is the
// acceptance test for the remainder of bean gosd-0vvh: an explicit
// `gosd build --board=rock-4se`, using --artifacts-dir to supply fake
// bootloader/kernel files, produces an image with idbloader.img and
// u-boot.itb raw-written at their locked offsets ahead of the boot
// partition, and a boot partition containing the kernel, DTB, initramfs,
// and a rendered extlinux.conf - the same shape as the Radxa Zero 3E and
// NanoPi Zero2. rock-4se is public since bean gosd-h8a8's activation, so
// it's also covered by the default all-boards build
// (TestBuildWithNoBoardFlagBuildsAllBoards) and by catalog output
// (TestBuildCatalogForRock4SEWritesEntry).
func TestBuildProducesABootableImageForRock4SEFromFakeArtifacts(t *testing.T) {
	origTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		t.Errorf("unexpected network request to %s during a --artifacts-dir build", r.URL)
		return nil, errors.New("network access is disabled in this test")
	})
	t.Cleanup(func() { http.DefaultTransport = origTransport })

	imgPath := filepath.Join(t.TempDir(), "hello-rock-4se.img")

	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"build", "../../examples/hello",
		"--board", "rock-4se",
		"--artifacts-dir", "testdata/fake-artifacts",
		"-o", imgPath,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("gosd build --board=rock-4se failed: %v", err)
	}

	assertRawWriteAt(t, imgPath, 32768, "fake idbloader.img")
	assertRawWriteAt(t, imgPath, 8388608, "fake u-boot.itb")

	d, err := diskfs.Open(imgPath, diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		t.Fatalf("reopening the built image failed: %v", err)
	}
	defer func() { _ = d.Close() }()

	fs, err := d.GetFilesystem(1)
	if err != nil {
		t.Fatalf("GetFilesystem(1) failed: %v", err)
	}

	for _, want := range []string{"Image", "rk3399-rock-4se.dtb", "initramfs.cpio.zst", "extlinux/extlinux.conf"} {
		if _, err := fs.ReadFile(want); err != nil {
			t.Errorf("boot partition is missing %q: %v", want, err)
		}
	}

	extlinuxConf, err := fs.ReadFile("extlinux/extlinux.conf")
	if err != nil {
		t.Fatalf("reading extlinux/extlinux.conf: %v", err)
	}
	wantExtlinuxConf := "default gosd\n" +
		"timeout 0\n" +
		"label gosd\n" +
		"    kernel /Image\n" +
		"    fdt /rk3399-rock-4se.dtb\n" +
		"    initrd /initramfs.cpio.zst\n" +
		"    append console=ttyS2,1500000n8 quiet init=/init gosd.board=rock-4se panic=10\n"
	if string(extlinuxConf) != wantExtlinuxConf {
		t.Errorf("extlinux.conf = %q, want %q", extlinuxConf, wantExtlinuxConf)
	}

	initramfsBytes, err := fs.ReadFile("initramfs.cpio.zst")
	if err != nil {
		t.Fatalf("reading initramfs.cpio.zst: %v", err)
	}
	assertCACertsBaked(t, decodeInitramfs(t, initramfsBytes))
}

// TestBuildCatalogForNanopiZero2WritesEntry confirms that, now that
// nanopi-zero2 is a public board (gosd-wskc's flip), --catalog on a
// nanopi-zero2-only build writes a real os_list.json entry - unlike
// qemu-virt (still internal-only, see
// TestBuildCatalogForQemuVirtOnlyWritesNothing above), and with its
// "devices" tag falling back to the raw board ID, matching how
// internal/catalog already handles the other non-Raspberry-Pi board
// (radxa-zero-3e): no official Imager device tag exists for either.
func TestBuildCatalogForNanopiZero2WritesEntry(t *testing.T) {
	origTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		t.Errorf("unexpected network request to %s during a --artifacts-dir build", r.URL)
		return nil, errors.New("network access is disabled in this test")
	})
	t.Cleanup(func() { http.DefaultTransport = origTransport })

	outDir := t.TempDir()
	imgPath := filepath.Join(outDir, "hello-nanopi-zero2.img")

	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"build", "../../examples/hello",
		"--board", "nanopi-zero2",
		"--artifacts-dir", "testdata/fake-artifacts",
		"--catalog",
		"--publish-base-url", "https://example.com/downloads",
		"-o", imgPath,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("gosd build --board=nanopi-zero2 --catalog failed: %v", err)
	}

	if _, err := os.Stat(imgPath); err != nil {
		t.Errorf("the image itself should be built: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(outDir, "hello-nanopi-zero2.os_list.json"))
	if err != nil {
		t.Fatalf("reading hello-nanopi-zero2.os_list.json: %v", err)
	}

	var list struct {
		OSList []struct {
			Name    string   `json:"name"`
			Devices []string `json:"devices"`
		} `json:"os_list"`
	}
	if err := json.Unmarshal(data, &list); err != nil {
		t.Fatalf("unmarshaling hello-nanopi-zero2.os_list.json: %v", err)
	}
	if len(list.OSList) != 1 {
		t.Fatalf("hello-nanopi-zero2.os_list.json has %d entries, want 1", len(list.OSList))
	}

	entry := list.OSList[0]
	if want := "hello (NanoPi Zero2)"; entry.Name != want {
		t.Errorf("name = %q, want %q", entry.Name, want)
	}
	if len(entry.Devices) != 1 || entry.Devices[0] != "nanopi-zero2" {
		t.Errorf("devices = %v, want [\"nanopi-zero2\"] (no official Imager tag for non-Pi hardware)", entry.Devices)
	}
}

// TestBuildCatalogForRock4SEWritesEntry confirms that, now that rock-4se is
// a public board (gosd-h8a8's flip), --catalog on a rock-4se-only build
// writes a real os_list.json entry with the display name from
// internal/catalog and its "devices" tag falling back to the raw board ID,
// like the other non-Raspberry-Pi boards.
func TestBuildCatalogForRock4SEWritesEntry(t *testing.T) {
	origTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		t.Errorf("unexpected network request to %s during a --artifacts-dir build", r.URL)
		return nil, errors.New("network access is disabled in this test")
	})
	t.Cleanup(func() { http.DefaultTransport = origTransport })

	outDir := t.TempDir()
	imgPath := filepath.Join(outDir, "hello-rock-4se.img")

	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"build", "../../examples/hello",
		"--board", "rock-4se",
		"--artifacts-dir", "testdata/fake-artifacts",
		"--catalog",
		"--publish-base-url", "https://example.com/downloads",
		"-o", imgPath,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("gosd build --board=rock-4se --catalog failed: %v", err)
	}

	if _, err := os.Stat(imgPath); err != nil {
		t.Errorf("the image itself should be built: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(outDir, "hello-rock-4se.os_list.json"))
	if err != nil {
		t.Fatalf("reading hello-rock-4se.os_list.json: %v", err)
	}

	var list struct {
		OSList []struct {
			Name    string   `json:"name"`
			Devices []string `json:"devices"`
		} `json:"os_list"`
	}
	if err := json.Unmarshal(data, &list); err != nil {
		t.Fatalf("unmarshaling hello-rock-4se.os_list.json: %v", err)
	}
	if len(list.OSList) != 1 {
		t.Fatalf("hello-rock-4se.os_list.json has %d entries, want 1", len(list.OSList))
	}
	entry := list.OSList[0]
	if want := "hello (Radxa ROCK 4SE)"; entry.Name != want {
		t.Errorf("name = %q, want %q", entry.Name, want)
	}
	if len(entry.Devices) != 1 || entry.Devices[0] != "rock-4se" {
		t.Errorf("devices = %v, want [\"rock-4se\"] (no official Imager tag for non-Pi hardware)", entry.Devices)
	}
}

// TestBuildProducesABootableImageForCubieA5EFromFakeArtifacts is the
// acceptance test for bean gosd-zh95: an explicit `gosd build
// --board=cubie-a5e`, using --artifacts-dir to supply fake bootloader/kernel
// files, produces an image with u-boot-sunxi-with-spl.bin raw-written at its
// locked offset (8KiB, the sunxi BootROM's single SPL+FIT load - unlike the
// Rockchip boards' idbloader/itb pair) ahead of the boot partition, and a
// boot partition containing the kernel, DTB, initramfs, and a rendered
// extlinux.conf. cubie-a5e is public since bean gosd-zh95's activation, so
// it's also covered by the default all-boards build
// (TestBuildWithNoBoardFlagBuildsAllBoards) and by catalog output
// (TestBuildCatalogForCubieA5EWritesEntry).
func TestBuildProducesABootableImageForCubieA5EFromFakeArtifacts(t *testing.T) {
	origTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		t.Errorf("unexpected network request to %s during a --artifacts-dir build", r.URL)
		return nil, errors.New("network access is disabled in this test")
	})
	t.Cleanup(func() { http.DefaultTransport = origTransport })

	imgPath := filepath.Join(t.TempDir(), "hello-cubie-a5e.img")

	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"build", "../../examples/hello",
		"--board", "cubie-a5e",
		"--artifacts-dir", "testdata/fake-artifacts",
		"-o", imgPath,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("gosd build --board=cubie-a5e failed: %v", err)
	}

	assertRawWriteAt(t, imgPath, 8192, "fake u-boot-sunxi-with-spl.bin")

	d, err := diskfs.Open(imgPath, diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		t.Fatalf("reopening the built image failed: %v", err)
	}
	defer func() { _ = d.Close() }()

	fs, err := d.GetFilesystem(1)
	if err != nil {
		t.Fatalf("GetFilesystem(1) failed: %v", err)
	}

	for _, want := range []string{"Image", "sun55i-a527-cubie-a5e.dtb", "initramfs.cpio.zst", "extlinux/extlinux.conf"} {
		if _, err := fs.ReadFile(want); err != nil {
			t.Errorf("boot partition is missing %q: %v", want, err)
		}
	}

	extlinuxConf, err := fs.ReadFile("extlinux/extlinux.conf")
	if err != nil {
		t.Fatalf("reading extlinux/extlinux.conf: %v", err)
	}
	wantExtlinuxConf := "default gosd\n" +
		"timeout 0\n" +
		"label gosd\n" +
		"    kernel /Image\n" +
		"    fdt /sun55i-a527-cubie-a5e.dtb\n" +
		"    initrd /initramfs.cpio.zst\n" +
		"    append console=ttyS0,115200n8 quiet init=/init gosd.board=cubie-a5e panic=10\n"
	if string(extlinuxConf) != wantExtlinuxConf {
		t.Errorf("extlinux.conf = %q, want %q", extlinuxConf, wantExtlinuxConf)
	}

	initramfsBytes, err := fs.ReadFile("initramfs.cpio.zst")
	if err != nil {
		t.Fatalf("reading initramfs.cpio.zst: %v", err)
	}
	assertCACertsBaked(t, decodeInitramfs(t, initramfsBytes))
}

// TestBuildCatalogForCubieA5EWritesEntry confirms that, now that cubie-a5e
// is a public board (gosd-zh95's flip), --catalog on a cubie-a5e-only build
// writes a real os_list.json entry with the display name from
// internal/catalog and its "devices" tag falling back to the raw board ID,
// like the other non-Raspberry-Pi boards.
// TestBuildWithUsbGadgetForCubieA5EShipsTheGadgetDTB is the acceptance test
// for bean gosd-3io0. On this board dr_mode="peripheral" in the stock DT is
// not sufficient for gadget mode: ehci0/ohci0 share the USB-C port's phy
// with the peripheral controller and win at probe, so an image must ship the
// variant DTB that disables them. --usb-gadget therefore changes which DTB
// lands on the boot partition and which one extlinux loads - the two cannot
// both be active on this hardware.
func TestBuildWithUsbGadgetForCubieA5EShipsTheGadgetDTB(t *testing.T) {
	origTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		t.Errorf("unexpected network request to %s during a --artifacts-dir build", r.URL)
		return nil, errors.New("network access is disabled in this test")
	})
	t.Cleanup(func() { http.DefaultTransport = origTransport })

	imgPath := filepath.Join(t.TempDir(), "hello-cubie-a5e.img")

	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"build", "../../examples/hello",
		"--board", "cubie-a5e",
		"--usb-gadget",
		"--artifacts-dir", "testdata/fake-artifacts",
		"-o", imgPath,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("gosd build --board=cubie-a5e --usb-gadget failed: %v", err)
	}

	d, err := diskfs.Open(imgPath, diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		t.Fatalf("reopening the built image failed: %v", err)
	}
	defer func() { _ = d.Close() }()

	fs, err := d.GetFilesystem(1)
	if err != nil {
		t.Fatalf("GetFilesystem(1) failed: %v", err)
	}

	gadgetDTB, err := fs.ReadFile("sun55i-a527-cubie-a5e-gadget.dtb")
	if err != nil {
		t.Fatalf("boot partition is missing the gadget DTB: %v", err)
	}
	if !strings.Contains(string(gadgetDTB), "gadget") {
		t.Errorf("gadget DTB content = %q, want the gadget variant's bytes", gadgetDTB)
	}

	if _, err := fs.ReadFile("sun55i-a527-cubie-a5e.dtb"); err == nil {
		t.Error("boot partition also carries the stock DTB; a gadget-mode image must ship only the variant, so there is no doubt which one is loaded")
	}

	extlinuxConf, err := fs.ReadFile("extlinux/extlinux.conf")
	if err != nil {
		t.Fatalf("reading extlinux/extlinux.conf: %v", err)
	}
	if !strings.Contains(string(extlinuxConf), "fdt /sun55i-a527-cubie-a5e-gadget.dtb\n") {
		t.Errorf("extlinux.conf = %q, want it to load the gadget DTB", extlinuxConf)
	}
}

func TestBuildCatalogForCubieA5EWritesEntry(t *testing.T) {
	origTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		t.Errorf("unexpected network request to %s during a --artifacts-dir build", r.URL)
		return nil, errors.New("network access is disabled in this test")
	})
	t.Cleanup(func() { http.DefaultTransport = origTransport })

	outDir := t.TempDir()
	imgPath := filepath.Join(outDir, "hello-cubie-a5e.img")

	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"build", "../../examples/hello",
		"--board", "cubie-a5e",
		"--artifacts-dir", "testdata/fake-artifacts",
		"--catalog",
		"--publish-base-url", "https://example.com/downloads",
		"-o", imgPath,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("gosd build --board=cubie-a5e --catalog failed: %v", err)
	}

	if _, err := os.Stat(imgPath); err != nil {
		t.Errorf("the image itself should be built: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(outDir, "hello-cubie-a5e.os_list.json"))
	if err != nil {
		t.Fatalf("reading hello-cubie-a5e.os_list.json: %v", err)
	}

	var list struct {
		OSList []struct {
			Name    string   `json:"name"`
			Devices []string `json:"devices"`
		} `json:"os_list"`
	}
	if err := json.Unmarshal(data, &list); err != nil {
		t.Fatalf("unmarshaling hello-cubie-a5e.os_list.json: %v", err)
	}
	if len(list.OSList) != 1 {
		t.Fatalf("hello-cubie-a5e.os_list.json has %d entries, want 1", len(list.OSList))
	}
	entry := list.OSList[0]
	if want := "hello (Radxa Cubie A5E)"; entry.Name != want {
		t.Errorf("name = %q, want %q", entry.Name, want)
	}
	if len(entry.Devices) != 1 || entry.Devices[0] != "cubie-a5e" {
		t.Errorf("devices = %v, want [\"cubie-a5e\"] (no official Imager tag for non-Pi hardware)", entry.Devices)
	}
}

// TestBuildCatalogWritesOsListJSON is the acceptance test for gosd-t6cs:
// `gosd build --catalog --publish-base-url=...` writes a combined
// os_list.json (and a per-image fragment) next to the built image, with the
// entry's extract_size/extract_sha256 matching the real .img file gosd just
// wrote.
func TestBuildCatalogWritesOsListJSON(t *testing.T) {
	origTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		t.Errorf("unexpected network request to %s during a --artifacts-dir build", r.URL)
		return nil, errors.New("network access is disabled in this test")
	})
	t.Cleanup(func() { http.DefaultTransport = origTransport })

	outDir := t.TempDir()
	imgPath := filepath.Join(outDir, "hello-pi-zero-2w.img")

	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"build", "../../examples/hello",
		"--board", "pi-zero-2w",
		"--artifacts-dir", "testdata/fake-artifacts",
		"--catalog",
		"--publish-base-url", "https://example.com/downloads",
		"-o", imgPath,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("gosd build failed: %v", err)
	}

	imgBytes, err := os.ReadFile(imgPath)
	if err != nil {
		t.Fatalf("reading built image: %v", err)
	}
	wantSum := sha256.Sum256(imgBytes)
	wantHex := hex.EncodeToString(wantSum[:])

	for _, listPath := range []string{
		filepath.Join(outDir, "os_list.json"),
		filepath.Join(outDir, "hello-pi-zero-2w.os_list.json"),
	} {
		data, err := os.ReadFile(listPath)
		if err != nil {
			t.Fatalf("reading %s: %v", listPath, err)
		}

		var list struct {
			OSList []struct {
				Name              string `json:"name"`
				URL               string `json:"url"`
				ExtractSize       int64  `json:"extract_size"`
				ExtractSHA256     string `json:"extract_sha256"`
				ImageDownloadSize int64  `json:"image_download_size"`
				InitFormat        string `json:"init_format"`
			} `json:"os_list"`
		}
		if err := json.Unmarshal(data, &list); err != nil {
			t.Fatalf("unmarshaling %s: %v", listPath, err)
		}
		if len(list.OSList) != 1 {
			t.Fatalf("%s has %d entries, want 1", listPath, len(list.OSList))
		}

		entry := list.OSList[0]
		if entry.URL != "https://example.com/downloads/hello-pi-zero-2w.img" {
			t.Errorf("%s: url = %q, want the joined base-url + filename", listPath, entry.URL)
		}
		if entry.ExtractSize != int64(len(imgBytes)) {
			t.Errorf("%s: extract_size = %d, want %d (the real image size)", listPath, entry.ExtractSize, len(imgBytes))
		}
		if entry.ImageDownloadSize != int64(len(imgBytes)) {
			t.Errorf("%s: image_download_size = %d, want %d", listPath, entry.ImageDownloadSize, len(imgBytes))
		}
		if entry.ExtractSHA256 != wantHex {
			t.Errorf("%s: extract_sha256 = %q, want %q (the real image's hash)", listPath, entry.ExtractSHA256, wantHex)
		}
		if entry.InitFormat != "cloudinit" {
			t.Errorf("%s: init_format = %q, want %q", listPath, entry.InitFormat, "cloudinit")
		}
	}
}

// TestBuildCatalogWithoutBaseURLFailsActionably confirms --catalog refuses
// to run without --publish-base-url, per its locked requirement, instead of
// building images it can't produce usable download links for.
func TestBuildCatalogWithoutBaseURLFailsActionably(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"build", "../../examples/hello",
		"--board", "pi-zero-2w",
		"--artifacts-dir", "testdata/fake-artifacts",
		"--catalog",
		"-o", filepath.Join(t.TempDir(), "hello-pi-zero-2w.img"),
	})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("gosd build --catalog with no --publish-base-url succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "--publish-base-url") {
		t.Errorf("error = %q, want it to mention --publish-base-url", err.Error())
	}
}

// buildConfigJSON runs `gosd build` for pi-zero-2w with extraArgs appended
// to the fixture flags every other build_integration_test.go test shares
// (no network, fake artifacts), and returns the resulting config.json,
// parsed.
func buildConfigJSON(t *testing.T, imgPath string, extraArgs ...string) initcfg.Config {
	t.Helper()

	args := append([]string{
		"build", "../../examples/hello",
		"--board", "pi-zero-2w",
		"--artifacts-dir", "testdata/fake-artifacts",
		"-o", imgPath,
	}, extraArgs...)

	cmd := newRootCmd()
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("gosd build failed: %v", err)
	}

	d, err := diskfs.Open(imgPath, diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		t.Fatalf("reopening the built image failed: %v", err)
	}
	defer func() { _ = d.Close() }()

	fs, err := d.GetFilesystem(1)
	if err != nil {
		t.Fatalf("GetFilesystem(1) failed: %v", err)
	}
	initramfsBytes, err := fs.ReadFile("initramfs.cpio.zst")
	if err != nil {
		t.Fatalf("reading initramfs.cpio.zst: %v", err)
	}
	configJSON := recordContent(t, decodeInitramfs(t, initramfsBytes), "etc/gosd/config.json")

	var cfg initcfg.Config
	if err := json.Unmarshal(configJSON, &cfg); err != nil {
		t.Fatalf("config.json = %s is not valid JSON: %v", configJSON, err)
	}
	return cfg
}

// readConfigJSON parses the /etc/gosd/config.json baked into an image that
// has already been built.
func readConfigJSON(t *testing.T, imgPath string) initcfg.Config {
	t.Helper()

	configJSON := recordContent(t, readImageInitramfs(t, imgPath), "etc/gosd/config.json")
	var cfg initcfg.Config
	if err := json.Unmarshal(configJSON, &cfg); err != nil {
		t.Fatalf("config.json = %s is not valid JSON: %v", configJSON, err)
	}
	return cfg
}

// buildQemuVirtConfigJSON mirrors buildConfigJSON but builds for qemu-virt
// instead of pi-zero-2w. Originally required because pi-zero-2w's stock
// kernel didn't support ext4; now every board GoSD ships does (bean
// gosd-ssth), so the choice of qemu-virt here is incidental - kept as-is to
// avoid churn, not because pi-zero-2w would fail --data-filesystem=ext4.
func buildQemuVirtConfigJSON(t *testing.T, imgPath string, extraArgs ...string) initcfg.Config {
	t.Helper()

	args := append([]string{
		"build", "../../examples/hello",
		"--board", "qemu-virt",
		"--artifacts-dir", "testdata/fake-artifacts",
		"-o", imgPath,
	}, extraArgs...)

	cmd := newRootCmd()
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("gosd build failed: %v", err)
	}

	d, err := diskfs.Open(imgPath, diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		t.Fatalf("reopening the built image failed: %v", err)
	}
	defer func() { _ = d.Close() }()

	fs, err := d.GetFilesystem(1)
	if err != nil {
		t.Fatalf("GetFilesystem(1) failed: %v", err)
	}
	initramfsBytes, err := fs.ReadFile("initramfs.cpio.zst")
	if err != nil {
		t.Fatalf("reading initramfs.cpio.zst: %v", err)
	}
	configJSON := recordContent(t, decodeInitramfs(t, initramfsBytes), "etc/gosd/config.json")

	var cfg initcfg.Config
	if err := json.Unmarshal(configJSON, &cfg); err != nil {
		t.Fatalf("config.json = %s is not valid JSON: %v", configJSON, err)
	}
	return cfg
}

// TestBuildBakesImageIdentityIntoConfigJSON is the acceptance test for
// gosd-acdn (docs/design/upgrade-path.md §4): config.json carries a
// content-derived image identity - a hex SHA-256 digest, never a
// timestamp or a random id.
func TestBuildBakesImageIdentityIntoConfigJSON(t *testing.T) {
	imgPath := filepath.Join(t.TempDir(), "hello-pi-zero-2w.img")
	cfg := buildConfigJSON(t, imgPath)

	if len(cfg.Identity) != sha256.Size*2 {
		t.Fatalf("config.json's identity = %q (%d chars), want a %d-character hex SHA-256 digest", cfg.Identity, len(cfg.Identity), sha256.Size*2)
	}
	if _, err := hex.DecodeString(cfg.Identity); err != nil {
		t.Errorf("config.json's identity = %q is not hex: %v", cfg.Identity, err)
	}
}

// TestBuildIdentityIsReproducibleAcrossRebuilds is the acceptance test for
// gosd-acdn's core requirement: identical rebuilds from identical inputs
// produce identical identities, which is what keeps the qemu CI path
// deterministic. It's also what backs the reproducibility claim in
// internal/pipeline.Assemble's comment above its ComputeIdentity call:
// gosd build's own image bytes are NOT fully reproducible today (go-diskfs's
// FAT32 formatter stamps wall-clock directory-entry timestamps and a volume
// serial number - confirmed by building this same fixture twice and diffing
// the two .img files), but the identity is hashed from the payload before
// it ever reaches the FAT layer, so it comes out identical regardless.
func TestBuildIdentityIsReproducibleAcrossRebuilds(t *testing.T) {
	dir := t.TempDir()
	cfg1 := buildConfigJSON(t, filepath.Join(dir, "build1.img"))
	cfg2 := buildConfigJSON(t, filepath.Join(dir, "build2.img"))

	if cfg1.Identity == "" {
		t.Fatal("first build's identity is empty")
	}
	if cfg1.Identity != cfg2.Identity {
		t.Errorf("identical rebuilds produced different identities: %q vs %q", cfg1.Identity, cfg2.Identity)
	}
}

// TestBuildIdentityChangesWithBootPayloadContent confirms the identity
// really is content-derived by changing one of the hashed FAT-root files
// (cmdline.txt, via --console-baud - see
// TestBuildConsoleBaudOverridesPiCmdlineTxt) and checking the identity
// moves.
func TestBuildIdentityChangesWithBootPayloadContent(t *testing.T) {
	dir := t.TempDir()
	defaultBaud := buildConfigJSON(t, filepath.Join(dir, "default-baud.img"))
	overriddenBaud := buildConfigJSON(t, filepath.Join(dir, "overridden-baud.img"), "--console-baud", "9600")

	if defaultBaud.Identity == overriddenBaud.Identity {
		t.Errorf("identity stayed %q after --console-baud changed cmdline.txt's content, want it to change", defaultBaud.Identity)
	}
}

// TestBuildIdentityUnaffectedByDataExpand confirms the one genuine
// exception ComputeIdentity's docstring documents: --data-size=expand only
// sets config.json's DataExpand field (config.json is entirely excluded
// from the hashed payload, and DataExpand has no footprint in the config tree or
// anywhere else in the payload, unlike Hostname/Wifi/Env), so it's the one
// build flag that changes config.json without moving Identity.
func TestBuildIdentityUnaffectedByDataExpand(t *testing.T) {
	dir := t.TempDir()
	withoutExpand := buildConfigJSON(t, filepath.Join(dir, "no-expand.img"))
	withExpand := buildConfigJSON(t, filepath.Join(dir, "expand.img"), "--data-size", "expand")

	if !withExpand.DataExpand {
		t.Fatal("config.json's dataExpand is false after --data-size=expand")
	}
	if withoutExpand.Identity == "" {
		t.Fatal("the --data-size=expand build's identity is empty")
	}
	if withoutExpand.Identity != withExpand.Identity {
		t.Errorf("identity differed across builds that only differed by --data-size=expand: %q vs %q", withoutExpand.Identity, withExpand.Identity)
	}
}

// TestBuildIdentityUnaffectedByDataFlush is TestBuildIdentityUnaffectedByData
// Expand's counterpart for gosd-9m1k's --data-flush flag: it only sets
// config.json's DataFlush field (config.json is entirely excluded from the
// hashed payload, and DataFlush has no footprint in the config tree or anywhere
// else in the payload, unlike Hostname/Wifi/Env), so it's another build flag
// that changes config.json without moving Identity.
func TestBuildIdentityUnaffectedByDataFlush(t *testing.T) {
	dir := t.TempDir()
	withoutFlush := buildConfigJSON(t, filepath.Join(dir, "no-flush.img"))
	withFlush := buildConfigJSON(t, filepath.Join(dir, "flush.img"), "--data-flush")

	if !withFlush.DataFlush {
		t.Fatal("config.json's dataFlush is false after --data-flush")
	}
	if withoutFlush.DataFlush {
		t.Fatal("config.json's dataFlush is true without --data-flush")
	}
	if withoutFlush.Identity == "" {
		t.Fatal("the --data-flush build's identity is empty")
	}
	if withoutFlush.Identity != withFlush.Identity {
		t.Errorf("identity differed across builds that only differed by --data-flush: %q vs %q", withoutFlush.Identity, withFlush.Identity)
	}
}

// TestBuildIdentityUnaffectedByDataFilesystem is TestBuildIdentityUnaffected
// ByDataFlush's counterpart for bean gosd-95yu's --data-filesystem flag.
// Unlike DataFlush, this flag DOES change the data partition's on-card layout (see
// initcfg.Config.DataFilesystem's docstring for the full rationale), but
// config.json is still excluded from ComputeIdentity's hashed payload in its
// entirety, and DataFilesystem has no footprint anywhere else in that
// payload (nothing in the config tree, unlike a hostname or WiFi network) -
// so two builds
// differing only by --data-filesystem must still produce the same identity.
// Built for qemu-virt rather than pi-zero-2w (the other identity tests'
// board) purely via buildQemuVirtConfigJSON - see that helper's doc comment
// for why the choice of board here is now incidental rather than required.
func TestBuildIdentityUnaffectedByDataFilesystem(t *testing.T) {
	dir := t.TempDir()
	withoutExt4 := buildQemuVirtConfigJSON(t, filepath.Join(dir, "fat32.img"))
	withExt4 := buildQemuVirtConfigJSON(t, filepath.Join(dir, "ext4.img"),
		"--data-filesystem", "ext4", "--data-size", strconv.FormatInt(diskfmt.EXT4GoldenData.MinBytes(), 10))

	if withExt4.DataFilesystem != "ext4" {
		t.Fatal("config.json's dataFilesystem is not ext4 after --data-filesystem=ext4")
	}
	if withoutExt4.DataFilesystem != "fat32" {
		t.Fatalf("config.json's dataFilesystem = %q without --data-filesystem, want fat32", withoutExt4.DataFilesystem)
	}
	if withoutExt4.Identity == "" {
		t.Fatal("the fat32 build's identity is empty")
	}
	if withoutExt4.Identity != withExt4.Identity {
		t.Errorf("identity differed across builds that only differed by --data-filesystem: %q vs %q", withoutExt4.Identity, withExt4.Identity)
	}
}

// TestBuildIdentityUnaffectedByLabelPrefix is TestBuildIdentityUnaffected
// ByDataFilesystem's counterpart for --label-prefix, and holds for the same
// structural reason: the label reaches config.json and nowhere else in
// ComputeIdentity's hashed payload, and config.json is excluded from that
// payload in its entirety. So two builds differing only by --label-prefix
// produce the same identity, even though they produce differently labelled
// cards (a real on-disk-ABI difference, see initcfg.Config.DataLabel).
func TestBuildIdentityUnaffectedByLabelPrefix(t *testing.T) {
	dir := t.TempDir()
	defaultLabels := buildConfigJSON(t, filepath.Join(dir, "default.img"))
	customLabels := buildConfigJSON(t, filepath.Join(dir, "custom.img"), "--label-prefix", "web")

	if defaultLabels.DataLabel != helloDataLabel {
		t.Fatalf("config.json's dataLabel = %q without --label-prefix, want %q", defaultLabels.DataLabel, helloDataLabel)
	}
	if customLabels.DataLabel != "web-data" {
		t.Fatalf("config.json's dataLabel = %q after --label-prefix=web, want web-data", customLabels.DataLabel)
	}
	if defaultLabels.Identity == "" {
		t.Fatal("the default-label build's identity is empty")
	}
	if defaultLabels.Identity != customLabels.Identity {
		t.Errorf("identity differed across builds that only differed by --label-prefix: %q vs %q", defaultLabels.Identity, customLabels.Identity)
	}
}

// TestBuildBakesReportMetadataIntoConfigJSON is the acceptance test for bean
// gosd-my8e: the board's display name (derived from the selected board, not
// a flag), the app name (derived, not a flag), --app-version, and
// --support-url all reach config.json under their own fields, so a future
// LAST_FATAL_ERROR.md renderer has a data source for its device: line,
// image: line, and its "visit <support_url>" fallback text.
func TestBuildBakesReportMetadataIntoConfigJSON(t *testing.T) {
	imgPath := filepath.Join(t.TempDir(), "hello-pi-zero-2w.img")
	cfg := buildConfigJSON(t, imgPath, "--app-version", "1.4.2", "--support-url", "https://example.com/support")

	if cfg.BoardDisplayName != "Raspberry Pi Zero 2W" {
		t.Errorf("config.json's boardDisplayName = %q, want %q (pi-zero-2w's DisplayName)", cfg.BoardDisplayName, "Raspberry Pi Zero 2W")
	}
	if cfg.AppName != "hello" {
		t.Errorf("config.json's appName = %q, want %q (examples/hello's sanitized basename)", cfg.AppName, "hello")
	}
	if cfg.AppVersion != "1.4.2" {
		t.Errorf("config.json's appVersion = %q, want %q", cfg.AppVersion, "1.4.2")
	}
	if cfg.SupportURL != "https://example.com/support" {
		t.Errorf("config.json's supportURL = %q, want %q", cfg.SupportURL, "https://example.com/support")
	}
}

// TestBuildReportMetadataAppVersionAndSupportURLAreOptional confirms
// --app-version and --support-url can both be omitted: the board display
// name and app name are still baked in (neither is a flag - gosd build
// always resolves a board and an app name), but AppVersion and SupportURL
// stay empty rather than getting a fabricated value.
func TestBuildReportMetadataAppVersionAndSupportURLAreOptional(t *testing.T) {
	imgPath := filepath.Join(t.TempDir(), "hello-pi-zero-2w.img")
	cfg := buildConfigJSON(t, imgPath)

	if cfg.BoardDisplayName != "Raspberry Pi Zero 2W" {
		t.Errorf("config.json's boardDisplayName = %q, want %q even with no --app-version/--support-url", cfg.BoardDisplayName, "Raspberry Pi Zero 2W")
	}
	if cfg.AppName != "hello" {
		t.Errorf("config.json's appName = %q, want %q even with no --app-version/--support-url", cfg.AppName, "hello")
	}
	if cfg.AppVersion != "" {
		t.Errorf("config.json's appVersion = %q, want empty when --app-version was not passed", cfg.AppVersion)
	}
	if cfg.SupportURL != "" {
		t.Errorf("config.json's supportURL = %q, want empty when --support-url was not passed", cfg.SupportURL)
	}
}

// TestBuildRefusesAnInvalidSupportURL confirms --support-url is validated
// before any image is written: a value that isn't an absolute http(s) URL
// must fail the build with an actionable error naming the flag, not silently
// bake in a link a device's owner could never follow.
func TestBuildRefusesAnInvalidSupportURL(t *testing.T) {
	for name, badURL := range map[string]string{
		"no scheme":          "example.com/support",
		"non-http(s) scheme": "ftp://example.com/support",
		"no host":            "https://",
	} {
		t.Run(name, func(t *testing.T) {
			imgPath := filepath.Join(t.TempDir(), "hello-pi-zero-2w.img")

			cmd := newRootCmd()
			cmd.SetArgs([]string{
				"build", "../../examples/hello",
				"--board", "pi-zero-2w",
				"--artifacts-dir", "testdata/fake-artifacts",
				"--support-url", badURL,
				"-o", imgPath,
			})
			err := cmd.Execute()
			if err == nil {
				t.Fatalf("gosd build --support-url=%q succeeded, want a refusal", badURL)
			}
			if !strings.Contains(err.Error(), "--support-url") {
				t.Errorf("refusal %q does not mention --support-url", err)
			}
			if _, statErr := os.Stat(imgPath); !os.IsNotExist(statErr) {
				t.Errorf("gosd build wrote %s despite refusing --support-url=%q; the refusal must come first", imgPath, badURL)
			}
		})
	}
}

// TestBuildIdentityUnaffectedByReportMetadata is TestBuildIdentityUnaffected
// ByLabelPrefix's counterpart for --app-version/--support-url: both reach
// config.json alone, config.json is excluded from ComputeIdentity's hashed
// payload in its entirety, and neither flag has any footprint elsewhere in
// that payload (nothing in the config tree, unlike a hostname or WiFi
// network) - so two
// builds differing only by these flags must still produce the same
// identity. This is also the concrete proof behind the bean's "not on-card
// ABI" requirement: docs/design/upgrade-path.md's adoption gate never even
// sees config.json's report-metadata fields, let alone keys off them.
func TestBuildIdentityUnaffectedByReportMetadata(t *testing.T) {
	dir := t.TempDir()
	withoutMetadata := buildConfigJSON(t, filepath.Join(dir, "no-metadata.img"))
	withMetadata := buildConfigJSON(t, filepath.Join(dir, "metadata.img"), "--app-version", "1.4.2", "--support-url", "https://example.com/support")

	if withMetadata.AppVersion != "1.4.2" || withMetadata.SupportURL != "https://example.com/support" {
		t.Fatalf("config.json didn't carry the report metadata: %+v", withMetadata)
	}
	if withoutMetadata.Identity == "" {
		t.Fatal("the build with no report metadata has an empty identity")
	}
	if withoutMetadata.Identity != withMetadata.Identity {
		t.Errorf("identity differed across builds that only differed by --app-version/--support-url: %q vs %q", withoutMetadata.Identity, withMetadata.Identity)
	}
}

// TestBuildTimestampVariesButIdentityDoesNotAcrossRebuilds is gosd-0esw's
// reproducibility proof: config.json's buildTimestamp (timesync's clock
// floor - see internal/initcfg.Config.BuildTime) necessarily differs on
// every build, by design, and unlike --data-size=expand's DataExpand it
// deliberately has no footprint in the config tree or anywhere else in the
// hashed payload either (see ComputeIdentity's docstring and
// BuildTimestamp's own doc). Identity staying equal here, alongside a
// buildTimestamp that provably moved, is the strongest evidence that
// adding it never put a dent in
// TestBuildIdentityIsReproducibleAcrossRebuilds' guarantee.
func TestBuildTimestampVariesButIdentityDoesNotAcrossRebuilds(t *testing.T) {
	dir := t.TempDir()
	cfg1 := buildConfigJSON(t, filepath.Join(dir, "build1.img"))
	cfg2 := buildConfigJSON(t, filepath.Join(dir, "build2.img"))

	if cfg1.BuildTimestamp == "" {
		t.Fatal("first build's config.json has an empty buildTimestamp")
	}
	if cfg1.BuildTimestamp == cfg2.BuildTimestamp {
		t.Fatalf("buildTimestamp stayed %q across two separate builds, want it to differ", cfg1.BuildTimestamp)
	}
	if cfg1.Identity != cfg2.Identity {
		t.Errorf("identity differed across rebuilds that only differed by buildTimestamp: %q vs %q", cfg1.Identity, cfg2.Identity)
	}
}

// TestBuildWithPlaceholdersWritesAPatchableInjectManifest is the end-to-end
// acceptance test for the image-injection contract (gosd-49it): `gosd
// build --placeholder` writes a <image>.inject.json manifest whose reported
// byte ranges can be overwritten with same-length bytes via plain
// os.WriteAt (no FAT tooling) and read back patched at the FAT level -
// exactly what a browser-side provisioning tool (docs/image-injection.md)
// does.
func TestBuildWithPlaceholdersWritesAPatchableInjectManifest(t *testing.T) {
	origTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		t.Errorf("unexpected network request to %s during a --artifacts-dir build", r.URL)
		return nil, errors.New("network access is disabled in this test")
	})
	t.Cleanup(func() { http.DefaultTransport = origTransport })

	imgPath := filepath.Join(t.TempDir(), "hello-pi-zero-2w.img")

	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"build", "../../examples/hello",
		"--board", "pi-zero-2w",
		"--artifacts-dir", "testdata/fake-artifacts",
		"--placeholder", "backupist.yaml=32KiB",
		"--placeholder", "network-config=4KiB",
		"-o", imgPath,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("gosd build failed: %v", err)
	}

	pristineImg, err := os.ReadFile(imgPath)
	if err != nil {
		t.Fatalf("reading the built image: %v", err)
	}
	wantImgSum := sha256.Sum256(pristineImg)

	manifestPath := inject.ManifestPath(imgPath)
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("reading the injection manifest %s: %v", manifestPath, err)
	}
	var manifest inject.Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatalf("manifest is not valid JSON: %v", err)
	}

	if manifest.GosdInject != 1 {
		t.Errorf("gosd_inject = %d, want 1", manifest.GosdInject)
	}
	if manifest.Board != "pi-zero-2w" {
		t.Errorf("board = %q, want pi-zero-2w", manifest.Board)
	}
	if manifest.Image.Filename != filepath.Base(imgPath) {
		t.Errorf("image.filename = %q, want %q", manifest.Image.Filename, filepath.Base(imgPath))
	}
	if manifest.Image.Size != int64(len(pristineImg)) {
		t.Errorf("image.size = %d, want %d", manifest.Image.Size, len(pristineImg))
	}
	if manifest.Image.SHA256 != hex.EncodeToString(wantImgSum[:]) {
		t.Errorf("image.sha256 = %q, want %q", manifest.Image.SHA256, hex.EncodeToString(wantImgSum[:]))
	}
	if len(manifest.Placeholders) != 2 {
		t.Fatalf("len(placeholders) = %d, want 2", len(manifest.Placeholders))
	}

	imgFile, err := os.OpenFile(imgPath, os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("opening the image for range reads/patches: %v", err)
	}
	defer func() { _ = imgFile.Close() }()

	var allRanges []inject.Range
	for _, p := range manifest.Placeholders {
		pristineContent := readRangesAt(t, imgFile, p.Ranges)
		gotSum := sha256.Sum256(pristineContent)
		if hex.EncodeToString(gotSum[:]) != p.SHA256 {
			t.Errorf("placeholder %q: content at its reported ranges hashes to %x, want its manifest sha256 %s", p.Path, gotSum, p.SHA256)
		}
		if !strings.HasPrefix(string(pristineContent), "# GOSD-PLACEHOLDER v1 path=") {
			t.Errorf("placeholder %q: content at its reported ranges = %q, want it to start with the documented header", p.Path, pristineContent[:min(len(pristineContent), 40)])
		}
		allRanges = append(allRanges, p.Ranges...)
	}
	assertRangesDontOverlap(t, allRanges)

	patched := map[string][]byte{
		"backupist.yaml": bytes.Repeat([]byte("BACKUPIST-PATCH-"), (32*1024/16)+1)[:32*1024],
		"network-config": bytes.Repeat([]byte("NETCFG-PATCH-"), (4*1024/13)+1)[:4*1024],
	}
	for _, p := range manifest.Placeholders {
		writeRangesAt(t, imgFile, p.Ranges, patched[p.Path])
	}
	if err := imgFile.Close(); err != nil {
		t.Fatalf("closing the image after patching: %v", err)
	}

	d, err := diskfs.Open(imgPath, diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		t.Fatalf("reopening the patched image failed: %v", err)
	}
	defer func() { _ = d.Close() }()

	fs, err := d.GetFilesystem(1)
	if err != nil {
		t.Fatalf("GetFilesystem(1) failed: %v", err)
	}
	for path, want := range patched {
		got, err := fs.ReadFile(path)
		if err != nil {
			t.Fatalf("reading patched %s back at the FAT level: %v", path, err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("patched FAT-level content of %s does not match the patch bytes written via os.WriteAt", path)
		}
	}
}

// readRangesAt reads and concatenates ranges from f, in order.
func readRangesAt(t *testing.T, f *os.File, ranges []inject.Range) []byte {
	t.Helper()
	var out []byte
	for _, r := range ranges {
		buf := make([]byte, r.Length)
		if _, err := f.ReadAt(buf, r.Offset); err != nil {
			t.Fatalf("ReadAt(offset=%d, len=%d): %v", r.Offset, r.Length, err)
		}
		out = append(out, buf...)
	}
	return out
}

// writeRangesAt slices content across ranges, in order, and writes each
// slice to f via plain WriteAt - exactly the splice a browser-side
// provisioning tool performs, with no FAT code involved.
func writeRangesAt(t *testing.T, f *os.File, ranges []inject.Range, content []byte) {
	t.Helper()
	var consumed int64
	for _, r := range ranges {
		if _, err := f.WriteAt(content[consumed:consumed+r.Length], r.Offset); err != nil {
			t.Fatalf("WriteAt(offset=%d, len=%d): %v", r.Offset, r.Length, err)
		}
		consumed += r.Length
	}
}

// assertRangesDontOverlap fails the test if any two ranges in ranges
// intersect.
func assertRangesDontOverlap(t *testing.T, ranges []inject.Range) {
	t.Helper()
	for i := 0; i < len(ranges); i++ {
		iEnd := ranges[i].Offset + ranges[i].Length
		for j := i + 1; j < len(ranges); j++ {
			jEnd := ranges[j].Offset + ranges[j].Length
			if ranges[i].Offset < jEnd && ranges[j].Offset < iEnd {
				t.Errorf("ranges overlap: [%d, %d) and [%d, %d)", ranges[i].Offset, iEnd, ranges[j].Offset, jEnd)
			}
		}
	}
}

// TestBuildCreatesMissingMultiBoardOutputDirectory is the regression test
// for the bug JP hit: `gosd build -o <dir>` for more than one board used to
// fail with "no such file or directory" the moment <dir> didn't already
// exist. -o naming a directory should get that directory created for you,
// per the principle of least surprise.
func TestBuildCreatesMissingMultiBoardOutputDirectory(t *testing.T) {
	origTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		t.Errorf("unexpected network request to %s during a --artifacts-dir build", r.URL)
		return nil, errors.New("network access is disabled in this test")
	})
	t.Cleanup(func() { http.DefaultTransport = origTransport })

	outDir := filepath.Join(t.TempDir(), "does", "not", "exist", "yet")

	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"build", "../../examples/hello",
		"--board", "pi-zero-2w",
		"--board", "radxa-zero-3e",
		"--artifacts-dir", "testdata/fake-artifacts",
		"-o", outDir,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("gosd build -o <missing directory> failed: %v", err)
	}

	for _, want := range []string{"hello-pi-zero-2w.img", "hello-radxa-zero-3e.img"} {
		if info, err := os.Stat(filepath.Join(outDir, want)); err != nil || info.Size() == 0 {
			t.Errorf("expected non-empty output image %q, got stat error %v", want, err)
		}
	}
}

// TestBuildCreatesMissingSingleBoardOutputParentDirectory covers the
// single-board case of the same bug: -o names the .img file directly, but
// its parent directory may not exist yet either.
func TestBuildCreatesMissingSingleBoardOutputParentDirectory(t *testing.T) {
	origTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		t.Errorf("unexpected network request to %s during a --artifacts-dir build", r.URL)
		return nil, errors.New("network access is disabled in this test")
	})
	t.Cleanup(func() { http.DefaultTransport = origTransport })

	imgPath := filepath.Join(t.TempDir(), "does", "not", "exist", "yet", "hello.img")

	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"build", "../../examples/hello",
		"--board", "pi-zero-2w",
		"--artifacts-dir", "testdata/fake-artifacts",
		"-o", imgPath,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("gosd build -o <file in missing directory> failed: %v", err)
	}

	if info, err := os.Stat(imgPath); err != nil || info.Size() == 0 {
		t.Errorf("expected non-empty output image at %q, got stat error %v", imgPath, err)
	}
}

// TestBuildMultiBoardOutputAsExistingFileFailsActionably confirms that
// pointing -o at a path that already exists as a plain file, when building
// more than one board, fails fast with an actionable error instead of the
// raw "no such file or directory"/"not a directory" error the underlying
// image writer would otherwise surface.
func TestBuildMultiBoardOutputAsExistingFileFailsActionably(t *testing.T) {
	outPath := filepath.Join(t.TempDir(), "already-a-file")
	if err := os.WriteFile(outPath, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("writing fixture file: %v", err)
	}

	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"build", "../../examples/hello",
		"--board", "pi-zero-2w",
		"--board", "radxa-zero-3e",
		"--artifacts-dir", "testdata/fake-artifacts",
		"-o", outPath,
	})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("gosd build -o <existing file> for multiple boards succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "-o must be a directory when building multiple boards") {
		t.Errorf("error = %q, want it to explain that -o must be a directory", err.Error())
	}
}

// assertRawWriteAt reads want's length worth of bytes from imgPath at
// offset and fails the test if they don't match want exactly.
func assertRawWriteAt(t *testing.T, imgPath string, offset int64, want string) {
	t.Helper()

	f, err := os.Open(imgPath)
	if err != nil {
		t.Fatalf("opening %s: %v", imgPath, err)
	}
	defer func() { _ = f.Close() }()

	got := make([]byte, len(want))
	if _, err := f.ReadAt(got, offset); err != nil {
		t.Fatalf("reading %d bytes at offset %d: %v", len(want), offset, err)
	}
	if string(got) != want {
		t.Errorf("raw bytes at offset %d = %q, want %q", offset, got, want)
	}
}

func decodeInitramfs(t *testing.T, compressed []byte) []cpio.Record {
	t.Helper()

	zr, err := zstd.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatalf("creating zstd reader: %v", err)
	}
	defer zr.Close()

	raw, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("decompressing initramfs: %v", err)
	}

	records, err := cpio.ReadAllRecords(cpio.Newc.Reader(bytes.NewReader(raw)))
	if err != nil {
		t.Fatalf("reading cpio records: %v", err)
	}
	return records
}

func hasRecord(records []cpio.Record, name string) bool {
	for _, r := range records {
		if r.Name == name {
			return true
		}
	}
	return false
}

func recordNames(records []cpio.Record) []string {
	names := make([]string, len(records))
	for i, r := range records {
		names[i] = r.Name
	}
	return names
}

func recordContent(t *testing.T, records []cpio.Record, name string) []byte {
	t.Helper()
	for _, r := range records {
		if r.Name != name {
			continue
		}
		got := make([]byte, r.FileSize)
		if _, err := r.ReadAt(got, 0); err != nil && err != io.EOF {
			t.Fatalf("reading record %q content: %v", name, err)
		}
		return got
	}
	t.Fatalf("no record named %q found in initramfs", name)
	return nil
}

// TestBuildWritesTheConfigTreeOntoTheBootPartition is the acceptance test
// for epic gosd-rw6n's build half: every image carries gosd's config tree
// at the FAT root, one setting per file, each padded to its reservation and
// each documented by a sidecar the card's owner can read - and nothing
// documents a feature this image doesn't carry.
func TestBuildWritesTheConfigTreeOntoTheBootPartition(t *testing.T) {
	noNetworkTransport(t)

	imgPath := filepath.Join(t.TempDir(), "hello-pi-zero-2w.img")
	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"build", "../../examples/hello",
		"--board", "pi-zero-2w",
		"--artifacts-dir", "testdata/fake-artifacts",
		"-o", imgPath,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("gosd build failed: %v", err)
	}

	d, err := diskfs.Open(imgPath, diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		t.Fatalf("reopening the built image failed: %v", err)
	}
	defer func() { _ = d.Close() }()
	fs, err := d.GetFilesystem(1)
	if err != nil {
		t.Fatalf("GetFilesystem(1) failed: %v", err)
	}

	ssid, err := fs.ReadFile("config/wifi/ssid")
	if err != nil {
		t.Fatalf("reading config/wifi/ssid off the card: %v", err)
	}
	if len(ssid) != configtree.MinValueBytes {
		t.Errorf("config/wifi/ssid is %d bytes, want the %d-byte reservation", len(ssid), configtree.MinValueBytes)
	}
	if got := configtree.TrimValue(ssid); got != "" {
		t.Errorf("config/wifi/ssid reads as %q, want unset", got)
	}

	for _, doc := range []string{"config/explain.md", "config/wifi/ssid.explain.md", "config/env/explain.md"} {
		content, err := fs.ReadFile(doc)
		if err != nil {
			t.Errorf("reading %s off the card: %v", doc, err)
			continue
		}
		if !bytes.HasPrefix(content, []byte("# ")) {
			t.Errorf("%s doesn't start with a markdown heading; it is the only documentation a card's owner gets", doc)
		}
	}

	// No --ingress, so nothing on the card may offer to configure one.
	if _, err := fs.ReadFile("config/ingress/cloudflared/token"); err == nil {
		t.Error("a build with no --ingress still wrote config/ingress/cloudflared/token")
	}
}

// TestBuildIngressWritesOnlyTheSelectedAgentsSettings pins the pruning rule
// the other way round: the agent baked into the image gets its settings
// directory, and the one that wasn't selected gets nothing.
func TestBuildIngressWritesOnlyTheSelectedAgentsSettings(t *testing.T) {
	noNetworkTransport(t)

	imgPath := filepath.Join(t.TempDir(), "hello-pi-zero-2w.img")
	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"build", "../../examples/hello",
		"--board", "pi-zero-2w",
		"--artifacts-dir", "testdata/fake-artifacts",
		"--ingress", "cloudflared",
		"-o", imgPath,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("gosd build --ingress cloudflared failed: %v", err)
	}

	d, err := diskfs.Open(imgPath, diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		t.Fatalf("reopening the built image failed: %v", err)
	}
	defer func() { _ = d.Close() }()
	fs, err := d.GetFilesystem(1)
	if err != nil {
		t.Fatalf("GetFilesystem(1) failed: %v", err)
	}

	token, err := fs.ReadFile("config/ingress/cloudflared/token")
	if err != nil {
		t.Fatalf("reading config/ingress/cloudflared/token off the card: %v", err)
	}
	if len(token) <= configtree.MinValueBytes {
		t.Errorf("the tunnel token reserves only %d bytes; a real Cloudflare token needs more room than the minimum", len(token))
	}
	if _, err := fs.ReadFile("config/ingress/tailscale-funnel/authkey"); err == nil {
		t.Error("--ingress cloudflared also wrote tailscale-funnel's settings")
	}
}

// writeConfigOverlay creates an app-side --config-dir with the given
// tree-relative files.
func writeConfigOverlay(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for p, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// TestBuildConfigDirIsInjectableAndBakedIntoConfigJSON is the acceptance
// test for the injection half: an app's own setting reaches the card, is
// published in the .inject.json manifest with its pristine bytes and value,
// can be overwritten with a plain os.WriteAt in the downloaded .img, reads
// back patched at the FAT level, and is digested in config.json so the
// device can tell a filled-in setting from a shipped one.
func TestBuildConfigDirIsInjectableAndBakedIntoConfigJSON(t *testing.T) {
	noNetworkTransport(t)

	overlay := writeConfigOverlay(t, map[string]string{
		"env/API_TOKEN":            "",
		"env/API_TOKEN.explain.md": "# API token\n\nThe token this app talks to its server with.\n",
		"hostname":                 "kitchen-clock\n",
	})

	imgPath := filepath.Join(t.TempDir(), "hello-pi-zero-2w.img")
	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"build", "../../examples/hello",
		"--board", "pi-zero-2w",
		"--artifacts-dir", "testdata/fake-artifacts",
		"--config-dir", overlay,
		"-o", imgPath,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("gosd build --config-dir failed: %v", err)
	}

	manifestData, err := os.ReadFile(inject.ManifestPath(imgPath))
	if err != nil {
		t.Fatalf("reading the injection manifest: %v", err)
	}
	var manifest inject.Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatalf("manifest is not valid JSON: %v", err)
	}

	byPath := make(map[string]inject.ConfigInfo, len(manifest.Config))
	for _, c := range manifest.Config {
		byPath[c.Path] = c
	}
	if got := byPath["hostname"].Value; got != "kitchen-clock" {
		t.Errorf("manifest publishes hostname's value as %q, want the app's own default", got)
	}
	token, ok := byPath["env/API_TOKEN"]
	if !ok {
		t.Fatalf("manifest has no env/API_TOKEN entry; it lists %v", manifest.Config)
	}
	if token.Value != "" {
		t.Errorf("env/API_TOKEN ships set to %q, want unset", token.Value)
	}

	// config.json's digests must describe exactly the bytes the manifest
	// publishes - that pairing is what lets the device recognize an
	// injected value at boot.
	config := readConfigJSON(t, imgPath)
	if got := config.ConfigDigests["env/API_TOKEN"]; got != token.SHA256 {
		t.Errorf("config.json digests env/API_TOKEN as %q, but the manifest publishes %q", got, token.SHA256)
	}

	imgFile, err := os.OpenFile(imgPath, os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("opening the image for range reads/patches: %v", err)
	}
	defer func() { _ = imgFile.Close() }()

	pristine := readRangesAt(t, imgFile, token.Ranges)
	gotSum := sha256.Sum256(pristine)
	if hex.EncodeToString(gotSum[:]) != token.SHA256 {
		t.Errorf("env/API_TOKEN's bytes at its published ranges hash to %x, want the manifest's %s", gotSum, token.SHA256)
	}

	injected := append([]byte("s3cret-token"), bytes.Repeat([]byte("\n"), int(token.Size)-len("s3cret-token"))...)
	writeRangesAt(t, imgFile, token.Ranges, injected)
	if err := imgFile.Close(); err != nil {
		t.Fatalf("closing the image after patching: %v", err)
	}

	d, err := diskfs.Open(imgPath, diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		t.Fatalf("reopening the patched image failed: %v", err)
	}
	defer func() { _ = d.Close() }()
	fs, err := d.GetFilesystem(1)
	if err != nil {
		t.Fatalf("GetFilesystem(1) failed: %v", err)
	}
	patched, err := fs.ReadFile("config/env/API_TOKEN")
	if err != nil {
		t.Fatalf("reading the patched setting back at the FAT level: %v", err)
	}
	if got := configtree.TrimValue(patched); got != "s3cret-token" {
		t.Errorf("config/env/API_TOKEN reads as %q at the FAT level, want the injected value", got)
	}
}

// TestBuildRefusesAnUndocumentedSetting: the explain.md gate is a build
// refusal, since a setting nobody can explain is worse than no setting.
func TestBuildRefusesAnUndocumentedSetting(t *testing.T) {
	noNetworkTransport(t)

	overlay := writeConfigOverlay(t, map[string]string{"env/API_TOKEN": ""})

	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"build", "../../examples/hello",
		"--board", "pi-zero-2w",
		"--artifacts-dir", "testdata/fake-artifacts",
		"--config-dir", overlay,
		"-o", filepath.Join(t.TempDir(), "hello-pi-zero-2w.img"),
	})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("gosd build with an undocumented setting succeeded, want a refusal")
	}
	if !strings.Contains(err.Error(), "explain.md") {
		t.Errorf("error = %q, want it to name the missing explain.md sidecar", err)
	}
}

// TestBuildIdentityChangesWithConfigTreeContent confirms the config tree is
// part of the hashed boot payload: two images differing only in a setting's
// baked value are genuinely different images, even though config.json
// itself is excluded from the payload (see ComputeIdentity's docstring).
func TestBuildIdentityChangesWithConfigTreeContent(t *testing.T) {
	dir := t.TempDir()
	overlayA := writeConfigOverlay(t, map[string]string{"hostname": "device-a\n"})
	overlayB := writeConfigOverlay(t, map[string]string{"hostname": "device-b\n"})

	deviceA := buildConfigJSON(t, filepath.Join(dir, "device-a.img"), "--config-dir", overlayA)
	deviceB := buildConfigJSON(t, filepath.Join(dir, "device-b.img"), "--config-dir", overlayB)

	if deviceA.Identity == "" {
		t.Fatal("device A's identity is empty")
	}
	if deviceA.Identity == deviceB.Identity {
		t.Errorf("identity stayed %q across builds whose config trees differ, want it to change", deviceA.Identity)
	}
}
