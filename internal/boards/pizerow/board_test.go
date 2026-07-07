package pizerow_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jphastings/gosd/internal/boards"
	"github.com/jphastings/gosd/internal/boards/pizerow"
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
	if got := pizerow.New().Name(); got != "pi-zero-w" {
		t.Errorf("Name() = %q, want pi-zero-w", got)
	}
}

func TestArch(t *testing.T) {
	got := pizerow.New().Arch()
	want := boards.Arch{GOARCH: "arm", GOARM: "6"}
	if got != want {
		t.Errorf("Arch() = %+v, want %+v (the Zero W's BCM2835 is 32-bit armv6 only)", got, want)
	}
}

func TestArtifactsIncludesKernelDTBAndManifestFiles(t *testing.T) {
	refs := pizerow.New().Artifacts()

	names := make(map[string]boards.ArtifactRef, len(refs))
	for _, r := range refs {
		names[r.Name] = r
	}

	for _, want := range []string{
		"kernel.img", "bcm2835-rpi-zero-w.dtb", "bootcode.bin", "start.elf", "fixup.dat",
		"cyfmac43430-sdio.bin", "cyfmac43430-sdio.clm_blob", "brcmfmac43430-sdio.txt",
	} {
		if _, ok := names[want]; !ok {
			t.Errorf("Artifacts() is missing %q", want)
		}
	}

	for _, noURL := range []string{"kernel.img", "bcm2835-rpi-zero-w.dtb"} {
		if ref := names[noURL]; ref.URL != "" {
			t.Errorf("%s has URL %q; it has no automatic fetch source yet and must come from --artifacts-dir only", noURL, ref.URL)
		}
	}

	for name, ref := range names {
		if name == "kernel.img" || name == "bcm2835-rpi-zero-w.dtb" {
			continue
		}
		if ref.URL == "" || ref.SHA256 == "" {
			t.Errorf("ArtifactRef %q is missing a pinned URL/SHA256: %+v", name, ref)
		}
	}
}

func TestBootFilesRequiresAnInitramfs(t *testing.T) {
	b := pizerow.New()
	art := resolveFakeArtifacts(t, b)

	if _, err := b.BootFiles(boards.BuildConfig{}, art); err == nil {
		t.Fatal("BootFiles() without an initramfs succeeded, want an error")
	}
}

func TestBootFilesContents(t *testing.T) {
	b := pizerow.New()
	art := resolveFakeArtifacts(t, b)
	art.Initramfs = strings.NewReader("fake initramfs bytes")

	files, err := b.BootFiles(boards.BuildConfig{}, art)
	if err != nil {
		t.Fatalf("BootFiles: %v", err)
	}

	for _, want := range []string{
		"kernel.img", "bcm2835-rpi-zero-w.dtb", "bootcode.bin", "start.elf", "fixup.dat",
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
	if !strings.Contains(string(cmdline), "gosd.board=pi-zero-w") {
		t.Errorf("cmdline.txt = %q, want it to contain gosd.board=pi-zero-w", cmdline)
	}

	configTxt, err := io.ReadAll(files["config.txt"])
	if err != nil {
		t.Fatalf("reading config.txt: %v", err)
	}
	if strings.Contains(string(configTxt), "arm_64bit") {
		t.Errorf("config.txt = %q, want no arm_64bit line (pi-zero-w is 32-bit only)", configTxt)
	}
	if !strings.Contains(string(configTxt), "kernel=kernel.img") {
		t.Errorf("config.txt = %q, want it to reference kernel.img", configTxt)
	}

	kernel, err := io.ReadAll(files["kernel.img"])
	if err != nil {
		t.Fatalf("reading kernel.img: %v", err)
	}
	if string(kernel) != "fake kernel.img" {
		t.Errorf("kernel.img content = %q, want the resolved artifact's content", kernel)
	}

	initramfs, err := io.ReadAll(files["initramfs.cpio.zst"])
	if err != nil {
		t.Fatalf("reading initramfs.cpio.zst: %v", err)
	}
	if string(initramfs) != "fake initramfs bytes" {
		t.Errorf("initramfs.cpio.zst content = %q, want the pipeline-built initramfs", initramfs)
	}
}

func TestBootFilesConfigTxtAddsUsbGadgetOverlayWhenRequested(t *testing.T) {
	b := pizerow.New()
	art := resolveFakeArtifacts(t, b)
	art.Initramfs = strings.NewReader("fake initramfs bytes")

	without, err := b.BootFiles(boards.BuildConfig{}, art)
	if err != nil {
		t.Fatalf("BootFiles() with UsbGadget=false: %v", err)
	}
	configWithout, err := io.ReadAll(without["config.txt"])
	if err != nil {
		t.Fatalf("reading config.txt: %v", err)
	}
	if strings.Contains(string(configWithout), "dtoverlay=dwc2") {
		t.Errorf("config.txt = %q, want no dwc2 overlay when --usb-gadget is not set", configWithout)
	}

	art.Initramfs = strings.NewReader("fake initramfs bytes")
	with, err := b.BootFiles(boards.BuildConfig{UsbGadget: true}, art)
	if err != nil {
		t.Fatalf("BootFiles() with UsbGadget=true: %v", err)
	}
	configWith, err := io.ReadAll(with["config.txt"])
	if err != nil {
		t.Fatalf("reading config.txt: %v", err)
	}
	if !strings.Contains(string(configWith), "dtoverlay=dwc2,dr_mode=peripheral") {
		t.Errorf("config.txt = %q, want the dwc2 peripheral-mode overlay when --usb-gadget is set", configWith)
	}
}

func TestRawWritesIsEmpty(t *testing.T) {
	if got := pizerow.New().RawWrites(boards.Artifacts{}); len(got) != 0 {
		t.Errorf("RawWrites() = %v, want empty: the Pi boots via the GPU ROM and FAT partition alone", got)
	}
}

func TestFirmwareFilesIncludesAliasesAsDuplicates(t *testing.T) {
	b := pizerow.New()
	art := resolveFakeArtifacts(t, b)

	files := b.FirmwareFiles(art)

	for _, want := range []string{
		"brcm/cyfmac43430-sdio.bin",
		"brcm/brcmfmac43430-sdio.raspberrypi,model-zero-w.bin",
		"brcm/brcmfmac43430-sdio.raspberrypi,model-zero-w.clm_blob",
		"brcm/brcmfmac43430-sdio.raspberrypi,model-zero-w.txt",
	} {
		if _, ok := files[want]; !ok {
			t.Errorf("FirmwareFiles() is missing %q; got keys %v", want, keys(files))
		}
	}

	base, err := io.ReadAll(files["brcm/cyfmac43430-sdio.bin"])
	if err != nil {
		t.Fatalf("reading base blob: %v", err)
	}
	alias, err := io.ReadAll(files["brcm/brcmfmac43430-sdio.raspberrypi,model-zero-w.bin"])
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
