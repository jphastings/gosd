package pizero2w_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jphastings/gosd/internal/boards"
	"github.com/jphastings/gosd/internal/boards/pizero2w"
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
	if got := pizero2w.New().Name(); got != "pi-zero-2w" {
		t.Errorf("Name() = %q, want pi-zero-2w", got)
	}
}

func TestArtifactsIncludesKernelDTBAndManifestFiles(t *testing.T) {
	refs := pizero2w.New().Artifacts()

	names := make(map[string]boards.ArtifactRef, len(refs))
	for _, r := range refs {
		names[r.Name] = r
	}

	for _, want := range []string{
		"kernel8.img", "bcm2710-rpi-zero-2-w.dtb", "bootcode.bin", "start.elf", "fixup.dat", "dwc2.dtbo",
		"brcmfmac43436-sdio.bin", "brcmfmac43436-sdio.clm_blob", "brcmfmac43436-sdio.txt",
		"brcmfmac43436s-sdio.bin", "brcmfmac43436s-sdio.txt",
	} {
		if _, ok := names[want]; !ok {
			t.Errorf("Artifacts() is missing %q", want)
		}
	}

	for _, noURL := range []string{"kernel8.img", "bcm2710-rpi-zero-2-w.dtb"} {
		if ref := names[noURL]; ref.URL != "" {
			t.Errorf("%s has URL %q; it has no automatic fetch source yet and must come from --artifacts-dir only", noURL, ref.URL)
		}
	}

	for name, ref := range names {
		if name == "kernel8.img" || name == "bcm2710-rpi-zero-2-w.dtb" {
			continue
		}
		if ref.URL == "" || ref.SHA256 == "" {
			t.Errorf("ArtifactRef %q is missing a pinned URL/SHA256: %+v", name, ref)
		}
	}
}

func TestBootFilesRequiresAnInitramfs(t *testing.T) {
	b := pizero2w.New()
	art := resolveFakeArtifacts(t, b)

	if _, err := b.BootFiles(boards.BuildConfig{}, art); err == nil {
		t.Fatal("BootFiles() without an initramfs succeeded, want an error")
	}
}

func TestBootFilesContents(t *testing.T) {
	b := pizero2w.New()
	art := resolveFakeArtifacts(t, b)
	art.Initramfs = strings.NewReader("fake initramfs bytes")

	files, err := b.BootFiles(boards.BuildConfig{}, art)
	if err != nil {
		t.Fatalf("BootFiles: %v", err)
	}

	for _, want := range []string{
		"kernel8.img", "bcm2710-rpi-zero-2-w.dtb", "bootcode.bin", "start.elf", "fixup.dat",
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
	if !strings.Contains(string(cmdline), "gosd.board=pi-zero-2w") {
		t.Errorf("cmdline.txt = %q, want it to contain gosd.board=pi-zero-2w", cmdline)
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

func TestBootFilesConfigTxtAddsUsbGadgetOverlayWhenRequested(t *testing.T) {
	b := pizero2w.New()
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
	if _, ok := without["overlays/dwc2.dtbo"]; ok {
		t.Error("BootFiles() ships overlays/dwc2.dtbo without --usb-gadget; it must ride only with config.txt's dtoverlay line")
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

	// The .dtbo itself must ship alongside the config.txt line: without it
	// on the boot partition, start.elf skips the directive silently and no
	// UDC ever appears (bean gosd-spjt).
	dtbo, ok := with["overlays/dwc2.dtbo"]
	if !ok {
		t.Fatal("BootFiles() with UsbGadget=true is missing overlays/dwc2.dtbo")
	}
	dtboBytes, err := io.ReadAll(dtbo)
	if err != nil {
		t.Fatalf("reading overlays/dwc2.dtbo: %v", err)
	}
	if string(dtboBytes) != "fake dwc2.dtbo" {
		t.Errorf("overlays/dwc2.dtbo content = %q, want the resolved artifact's content", dtboBytes)
	}
}

func TestBootFilesDefaultsConsoleBaudTo115200(t *testing.T) {
	b := pizero2w.New()
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
	b := pizero2w.New()
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
	if got := pizero2w.New().ConsoleBaudSupport(); !got.Supported {
		t.Errorf("ConsoleBaudSupport() = %+v, want Supported: true (cmdline.txt's console= rate is board-rendered)", got)
	}
}

func TestRawWritesIsEmpty(t *testing.T) {
	if got := pizero2w.New().RawWrites(boards.Artifacts{}); len(got) != 0 {
		t.Errorf("RawWrites() = %v, want empty: the Pi boots via the GPU ROM and FAT partition alone", got)
	}
}

func TestFirmwareFilesIncludesAliasesAsDuplicates(t *testing.T) {
	b := pizero2w.New()
	art := resolveFakeArtifacts(t, b)

	files := b.FirmwareFiles(art)

	for _, want := range []string{
		"brcm/brcmfmac43436-sdio.bin",
		"brcm/brcmfmac43436-sdio.raspberrypi,model-zero-2-w.bin",
		"brcm/brcmfmac43430b0-sdio.raspberrypi,model-zero-2-w.bin",
		"brcm/brcmfmac43430-sdio.raspberrypi,model-zero-2-w.bin",
	} {
		if _, ok := files[want]; !ok {
			t.Errorf("FirmwareFiles() is missing %q; got keys %v", want, keys(files))
		}
	}

	base, err := io.ReadAll(files["brcm/brcmfmac43436-sdio.bin"])
	if err != nil {
		t.Fatalf("reading base blob: %v", err)
	}
	alias, err := io.ReadAll(files["brcm/brcmfmac43436-sdio.raspberrypi,model-zero-2-w.bin"])
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

func TestUsbGadgetSupportIsSupported(t *testing.T) {
	if got := pizero2w.New().UsbGadgetSupport(); !got.Supported {
		t.Errorf("UsbGadgetSupport() = %+v, want Supported: true (dwc2 overlay puts the port into peripheral mode)", got)
	}
}

func TestEXT4SupportIsSupported(t *testing.T) {
	if got := pizero2w.New().EXT4Support(); !got.Supported {
		t.Errorf("EXT4Support() = %+v, want Supported: true (stock kernel builds CONFIG_EXT4_FS=y since artifacts v0.10.0)", got)
	}
}
