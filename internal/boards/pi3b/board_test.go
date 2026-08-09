package pi3b_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jphastings/gosd/internal/boards"
	"github.com/jphastings/gosd/internal/boards/pi3b"
)

// resolveFakeArtifacts seeds a temp --artifacts-dir with a fake file for
// every artifact the board asks for, then resolves it - exercising the same
// path gosd's integration test uses, without needing real firmware.
func resolveFakeArtifacts(t *testing.T, b boards.Board) boards.Artifacts {
	t.Helper()

	dir := t.TempDir()
	for _, ref := range b.Artifacts() {
		if err := os.WriteFile(filepath.Join(dir, ref.Name), []byte("fake "+ref.Name), 0o644); err != nil {
			t.Fatalf("seeding fake artifact %q: %v", ref.Name, err)
		}
	}

	art, err := boards.ResolveArtifacts(context.Background(), b.Name(), b.Artifacts(), dir, t.TempDir(), nil)
	if err != nil {
		t.Fatalf("ResolveArtifacts: %v", err)
	}
	return art
}

func TestName(t *testing.T) {
	if got := pi3b.New().Name(); got != "pi-3b" {
		t.Errorf("Name() = %q, want pi-3b", got)
	}
}

func TestArch(t *testing.T) {
	got := pi3b.New().Arch()
	want := boards.Arch{GOARCH: "arm64"}
	if got != want {
		t.Errorf("Arch() = %+v, want %+v (the 3B's BCM2837 is the same arm64 family as the Zero 2W)", got, want)
	}
}

func TestArtifactsIncludesKernelDTBAndManifestFiles(t *testing.T) {
	refs := pi3b.New().Artifacts()

	names := make(map[string]boards.ArtifactRef, len(refs))
	for _, r := range refs {
		names[r.Name] = r
	}

	// Both family DTBs ship in one image (the firmware picks by board
	// revision - the 3B+'s firmware asks for the -plus blob; bean gosd-oq0z).
	kernelBuilt := []string{"kernel8.img", "bcm2710-rpi-3-b.dtb", "bcm2710-rpi-3-b-plus.dtb"}

	for _, want := range append(kernelBuilt,
		"bootcode.bin", "start.elf", "fixup.dat",
		"cyfmac43430-sdio.bin", "cyfmac43430-sdio.clm_blob", "brcmfmac43430-sdio.txt",
	) {
		if _, ok := names[want]; !ok {
			t.Errorf("Artifacts() is missing %q", want)
		}
	}

	noURLNames := make(map[string]bool, len(kernelBuilt))
	for _, noURL := range kernelBuilt {
		noURLNames[noURL] = true
		if ref := names[noURL]; ref.URL != "" {
			t.Errorf("%s has URL %q; it is compiled by gosd build-kernel and must resolve from --artifacts-dir or the artifact release, never a pinned URL", noURL, ref.URL)
		}
	}

	for name, ref := range names {
		if noURLNames[name] {
			continue
		}
		if ref.URL == "" || ref.SHA256 == "" {
			t.Errorf("ArtifactRef %q is missing a pinned URL/SHA256: %+v", name, ref)
		}
	}
}

func TestBootFilesRequiresAnInitramfs(t *testing.T) {
	b := pi3b.New()
	art := resolveFakeArtifacts(t, b)

	if _, err := b.BootFiles(boards.BuildConfig{}, art); err == nil {
		t.Fatal("BootFiles() without an initramfs succeeded, want an error")
	}
}

func TestBootFilesContents(t *testing.T) {
	b := pi3b.New()
	art := resolveFakeArtifacts(t, b)
	art.Initramfs = strings.NewReader("fake initramfs bytes")

	files, err := b.BootFiles(boards.BuildConfig{}, art)
	if err != nil {
		t.Fatalf("BootFiles: %v", err)
	}

	for _, want := range []string{
		"kernel8.img", "bcm2710-rpi-3-b.dtb", "bcm2710-rpi-3-b-plus.dtb",
		"bootcode.bin", "start.elf", "fixup.dat",
		"config.txt", "cmdline.txt", "initramfs.cpio.zst",
	} {
		if _, ok := files[want]; !ok {
			t.Errorf("BootFiles() is missing %q", want)
		}
	}

	cmdline, err := io.ReadAll(files["cmdline.txt"])
	if err != nil {
		t.Fatalf("reading cmdline.txt: %v", err)
	}
	if !strings.Contains(string(cmdline), "gosd.board=pi-3b") {
		t.Errorf("cmdline.txt = %q, want it to contain gosd.board=pi-3b", cmdline)
	}

	configTxt, err := io.ReadAll(files["config.txt"])
	if err != nil {
		t.Fatalf("reading config.txt: %v", err)
	}
	if !strings.Contains(string(configTxt), "arm_64bit=1") {
		t.Errorf("config.txt = %q, want arm_64bit=1 (the 3B boots the arm64 kernel8.img)", configTxt)
	}
	if !strings.Contains(string(configTxt), "kernel=kernel8.img") {
		t.Errorf("config.txt = %q, want it to reference kernel8.img", configTxt)
	}

	kernel, err := io.ReadAll(files["kernel8.img"])
	if err != nil {
		t.Fatalf("reading kernel8.img: %v", err)
	}
	if string(kernel) != "fake kernel8.img" {
		t.Errorf("kernel8.img content = %q, want the resolved artifact's content", kernel)
	}

	initramfs, err := io.ReadAll(files["initramfs.cpio.zst"])
	if err != nil {
		t.Fatalf("reading initramfs.cpio.zst: %v", err)
	}
	if string(initramfs) != "fake initramfs bytes" {
		t.Errorf("initramfs.cpio.zst content = %q, want the pipeline-built initramfs", initramfs)
	}
}

func TestBootFilesConfigTxtNeverAddsAGadgetOverlay(t *testing.T) {
	b := pi3b.New()
	art := resolveFakeArtifacts(t, b)
	art.Initramfs = strings.NewReader("fake initramfs bytes")

	// Even a (mistakenly plumbed-through) UsbGadget build config must not
	// produce an overlay line: the hub-wired port can never be a
	// peripheral, and a dwc2 overlay would detach the board's Ethernet.
	files, err := b.BootFiles(boards.BuildConfig{UsbGadget: true}, art)
	if err != nil {
		t.Fatalf("BootFiles() with UsbGadget=true: %v", err)
	}
	configTxt, err := io.ReadAll(files["config.txt"])
	if err != nil {
		t.Fatalf("reading config.txt: %v", err)
	}
	if strings.Contains(string(configTxt), "dtoverlay") {
		t.Errorf("config.txt = %q, want no dtoverlay line ever on pi-3b", configTxt)
	}
}

func TestBootFilesDefaultsConsoleBaudTo115200(t *testing.T) {
	b := pi3b.New()
	art := resolveFakeArtifacts(t, b)
	art.Initramfs = strings.NewReader("fake initramfs bytes")

	files, err := b.BootFiles(boards.BuildConfig{}, art)
	if err != nil {
		t.Fatalf("BootFiles: %v", err)
	}
	cmdline, err := io.ReadAll(files["cmdline.txt"])
	if err != nil {
		t.Fatalf("reading cmdline.txt: %v", err)
	}
	if !strings.Contains(string(cmdline), "console=serial0,115200") {
		t.Errorf("cmdline.txt = %q, want it to default to console=serial0,115200 when ConsoleBaud is unset", cmdline)
	}
}

func TestBootFilesHonorsConsoleBaudOverride(t *testing.T) {
	b := pi3b.New()
	art := resolveFakeArtifacts(t, b)
	art.Initramfs = strings.NewReader("fake initramfs bytes")

	files, err := b.BootFiles(boards.BuildConfig{ConsoleBaud: 57600}, art)
	if err != nil {
		t.Fatalf("BootFiles: %v", err)
	}
	cmdline, err := io.ReadAll(files["cmdline.txt"])
	if err != nil {
		t.Fatalf("reading cmdline.txt: %v", err)
	}
	if !strings.Contains(string(cmdline), "console=serial0,57600") {
		t.Errorf("cmdline.txt = %q, want it to contain console=serial0,57600 when ConsoleBaud=57600", cmdline)
	}
	if strings.Contains(string(cmdline), "115200") {
		t.Errorf("cmdline.txt = %q, want the default 115200 rate gone once overridden", cmdline)
	}
}

func TestConsoleBaudSupportIsSupported(t *testing.T) {
	if got := pi3b.New().ConsoleBaudSupport(); !got.Supported {
		t.Errorf("ConsoleBaudSupport() = %+v, want Supported: true (cmdline.txt's console= rate is board-rendered)", got)
	}
}

func TestRawWritesIsEmpty(t *testing.T) {
	if got := pi3b.New().RawWrites(boards.Artifacts{}); len(got) != 0 {
		t.Errorf("RawWrites() = %v, want empty: the Pi boots via the GPU ROM and FAT partition alone", got)
	}
}

func TestFirmwareFilesIncludesAliasesAsDuplicates(t *testing.T) {
	b := pi3b.New()
	art := resolveFakeArtifacts(t, b)

	files := b.FirmwareFiles(art)

	for _, want := range []string{
		"brcm/cyfmac43430-sdio.bin",
		"brcm/brcmfmac43430-sdio.raspberrypi,3-model-b.bin",
		"brcm/brcmfmac43430-sdio.raspberrypi,3-model-b.clm_blob",
		"brcm/brcmfmac43430-sdio.raspberrypi,3-model-b.txt",
	} {
		if _, ok := files[want]; !ok {
			t.Errorf("FirmwareFiles() is missing %q; got keys %v", want, keys(files))
		}
	}

	base, err := io.ReadAll(files["brcm/cyfmac43430-sdio.bin"])
	if err != nil {
		t.Fatalf("reading base blob: %v", err)
	}
	alias, err := io.ReadAll(files["brcm/brcmfmac43430-sdio.raspberrypi,3-model-b.bin"])
	if err != nil {
		t.Fatalf("reading alias: %v", err)
	}
	if string(base) != string(alias) {
		t.Errorf("alias content = %q, want it to duplicate the base blob's content %q", alias, base)
	}
}

func keys(m map[string]io.Reader) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestUsbGadgetSupportIsUnsupportedWithReason(t *testing.T) {
	got := pi3b.New().UsbGadgetSupport()
	if got.Supported {
		t.Fatalf("UsbGadgetSupport() = %+v, want Supported: false (the 3B's USB is hard-wired through the LAN9514 hub)", got)
	}
	if !strings.Contains(got.Reason, "LAN9514") {
		t.Errorf("UsbGadgetSupport().Reason = %q, want it to explain the LAN9514 hub wiring", got.Reason)
	}
}

func TestEXT4SupportIsUnsupportedWithReason(t *testing.T) {
	got := pi3b.New().EXT4Support()
	if got.Supported {
		t.Fatalf("EXT4Support() = %+v, want Supported: false (the stock kernel has no CONFIG_EXT4_FS)", got)
	}
	if !strings.Contains(got.Reason, "CONFIG_EXT4_FS") {
		t.Errorf("EXT4Support().Reason = %q, want it to name the missing kernel option", got.Reason)
	}
}
