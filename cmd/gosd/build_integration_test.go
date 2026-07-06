package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	diskfs "github.com/diskfs/go-diskfs"
	"github.com/diskfs/go-diskfs/disk"
	"github.com/diskfs/go-diskfs/partition/mbr"
	"github.com/klauspost/compress/zstd"
	"github.com/u-root/u-root/pkg/cpio"
)

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
		"--hostname", "integration-test",
		"--wifi-ssid", "test-network",
		"--wifi-pass", "test-passphrase",
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
		"kernel8.img", "bootcode.bin", "start.elf", "fixup.dat",
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
	for _, want := range []string{`"board":"pi-zero-2w"`, `"hostname":"integration-test"`, `"ssid":"test-network"`, `"passphrase":"test-passphrase"`} {
		if !strings.Contains(string(configJSON), want) {
			t.Errorf("config.json = %q, want it to contain %q", configJSON, want)
		}
	}

	// With no --data-size flag, the default (1GiB) GOSD-DATA partition must
	// exist: FAT32 (MBR type 0x0C), starting immediately after GOSD-BOOT.
	// The 1GiB-plus image stays cheap because go-diskfs writes it sparsely.
	dataPart, err := d.GetPartition(2)
	if err != nil {
		t.Fatalf("GetPartition(2) failed: %v", err)
	}
	if got, want := dataPart.GetStart(), int64(272*1024*1024); got != want {
		t.Errorf("partition 2 starts at byte %d, want %d (immediately after GOSD-BOOT)", got, want)
	}
	if got, want := dataPart.GetSize(), int64(1024*1024*1024); got != want {
		t.Errorf("partition 2 size = %d bytes, want %d (the 1GiB default)", got, want)
	}
	assertMBRPartitionType(t, d, 2, mbr.Fat32LBA)

	dataFS, err := d.GetFilesystem(2)
	if err != nil {
		t.Fatalf("GetFilesystem(2) failed: %v", err)
	}
	if label := strings.TrimSpace(dataFS.Label()); label != "GOSD-DATA" {
		t.Errorf("data partition label = %q, want GOSD-DATA", label)
	}
}

// TestBuildWithDataSizeZeroOmitsTheDataPartition covers the explicit opt-out:
// --data-size=0 must produce the original single-partition layout.
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
		"--hostname", "integration-test",
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
		"    append console=ttyS2,1500000n8 quiet init=/init gosd.board=radxa-zero-3e\n"
	if string(extlinuxConf) != wantExtlinuxConf {
		t.Errorf("extlinux.conf = %q, want %q", extlinuxConf, wantExtlinuxConf)
	}
}

// TestBuildWithNoBoardFlagBuildsAllBoards confirms that omitting --board (as
// gosd's locked "no --board builds every board" decision requires) now
// produces both the pi-zero-2w and the radxa-zero-3e images, not just the
// former.
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

	for _, want := range []string{"hello-pi-zero-2w.img", "hello-radxa-zero-3e.img"} {
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

	// qemu-virt is internal-only: the default no---board build must produce
	// exactly the two public boards' images, never a third for it.
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
	if len(imgNames) != 2 {
		t.Errorf("default build produced %d .img files (%v), want exactly 2 (qemu-virt must stay excluded)", len(imgNames), imgNames)
	}
	if _, err := os.Stat(filepath.Join(outDir, "hello-qemu-virt.img")); err == nil {
		t.Error("default build produced hello-qemu-virt.img; qemu-virt is internal-only and must be excluded from the default build set")
	}
}

// TestBuildProducesAQemuVirtImageFromFakeArtifacts is the acceptance test for
// gosd-2v40: an explicit `gosd build --board=qemu-virt`, using
// --artifacts-dir to supply a fake kernel image, produces an image whose
// boot partition contains exactly the kernel (Image), the initramfs, and
// gosd.toml (added by the pipeline for every board) - no config.txt or
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
		"--hostname", "integration-test",
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

	for _, want := range []string{"Image", "initramfs.cpio.zst", "gosd.toml"} {
		if _, err := fs.ReadFile(want); err != nil {
			t.Errorf("boot partition is missing %q: %v", want, err)
		}
	}
	for _, absent := range []string{"config.txt", "cmdline.txt", "extlinux/extlinux.conf"} {
		if _, err := fs.ReadFile(absent); err == nil {
			t.Errorf("boot partition unexpectedly contains %q; qemu-virt has no on-device bootloader to configure", absent)
		}
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
