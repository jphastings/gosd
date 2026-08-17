package cubiea5e_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jphastings/gosd/internal/boards"
	"github.com/jphastings/gosd/internal/boards/cubiea5e"
)

// resolveFakeArtifacts seeds a temp --artifacts-dir with a fake file for
// every artifact the board asks for, then resolves it - exercising the same
// path gosd's integration test uses, without needing real bootloader/kernel
// builds.
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
	if got := cubiea5e.New().Name(); got != "cubie-a5e" {
		t.Errorf("Name() = %q, want cubie-a5e", got)
	}
}

func TestArtifactsHasNoAutomaticFetchSource(t *testing.T) {
	refs := cubiea5e.New().Artifacts()

	names := make(map[string]boards.ArtifactRef, len(refs))
	for _, r := range refs {
		names[r.Name] = r
	}

	for _, want := range []string{"u-boot-sunxi-with-spl.bin", "Image", "sun55i-a527-cubie-a5e.dtb"} {
		ref, ok := names[want]
		if !ok {
			t.Errorf("Artifacts() is missing %q", want)
			continue
		}
		if ref.URL != "" {
			t.Errorf("ArtifactRef %q has URL %q; it has no automatic fetch source yet and must come from --artifacts-dir only", want, ref.URL)
		}
	}
}

func TestBootFilesRequiresAnInitramfs(t *testing.T) {
	b := cubiea5e.New()
	art := resolveFakeArtifacts(t, b)

	if _, err := b.BootFiles(boards.BuildConfig{}, art); err == nil {
		t.Fatal("BootFiles() without an initramfs succeeded, want an error")
	}
}

func TestBootFilesContents(t *testing.T) {
	b := cubiea5e.New()
	art := resolveFakeArtifacts(t, b)
	art.Initramfs = strings.NewReader("fake initramfs bytes")

	files, err := b.BootFiles(boards.BuildConfig{}, art)
	if err != nil {
		t.Fatalf("BootFiles: %v", err)
	}

	for _, want := range []string{"Image", "sun55i-a527-cubie-a5e.dtb", "initramfs.cpio.zst", "extlinux/extlinux.conf"} {
		if _, ok := files[want]; !ok {
			t.Errorf("BootFiles() is missing %q", want)
		}
	}

	kernel, err := io.ReadAll(files["Image"])
	if err != nil {
		t.Fatalf("reading Image: %v", err)
	}
	if string(kernel) != "fake Image" {
		t.Errorf("Image content = %q, want the resolved artifact's content", kernel)
	}

	initramfs, err := io.ReadAll(files["initramfs.cpio.zst"])
	if err != nil {
		t.Fatalf("reading initramfs.cpio.zst: %v", err)
	}
	if string(initramfs) != "fake initramfs bytes" {
		t.Errorf("initramfs.cpio.zst content = %q, want the pipeline-built initramfs", initramfs)
	}

	extlinuxConf, err := io.ReadAll(files["extlinux/extlinux.conf"])
	if err != nil {
		t.Fatalf("reading extlinux/extlinux.conf: %v", err)
	}
	if !strings.Contains(string(extlinuxConf), "gosd.board=cubie-a5e") {
		t.Errorf("extlinux.conf = %q, want it to contain gosd.board=cubie-a5e", extlinuxConf)
	}
}

func TestBootFilesIgnoresUsbGadget(t *testing.T) {
	b := cubiea5e.New()
	art := resolveFakeArtifacts(t, b)
	art.Initramfs = strings.NewReader("fake initramfs bytes")
	without, err := b.BootFiles(boards.BuildConfig{}, art)
	if err != nil {
		t.Fatalf("BootFiles() with UsbGadget=false: %v", err)
	}

	art.Initramfs = strings.NewReader("fake initramfs bytes")
	with, err := b.BootFiles(boards.BuildConfig{UsbGadget: true}, art)
	if err != nil {
		t.Fatalf("BootFiles() with UsbGadget=true: %v", err)
	}

	extlinuxWithout, err := io.ReadAll(without["extlinux/extlinux.conf"])
	if err != nil {
		t.Fatalf("reading extlinux.conf: %v", err)
	}
	extlinuxWith, err := io.ReadAll(with["extlinux/extlinux.conf"])
	if err != nil {
		t.Fatalf("reading extlinux.conf: %v", err)
	}
	if string(extlinuxWithout) != string(extlinuxWith) {
		t.Errorf("extlinux.conf differs between UsbGadget=false/true; this board needs no boot-time change for USB gadget mode")
	}
}

func TestBootFilesDefaultsConsoleBaudTo115200(t *testing.T) {
	b := cubiea5e.New()
	art := resolveFakeArtifacts(t, b)
	art.Initramfs = strings.NewReader("fake initramfs bytes")

	files, err := b.BootFiles(boards.BuildConfig{}, art)
	if err != nil {
		t.Fatalf("BootFiles: %v", err)
	}
	extlinuxConf, err := io.ReadAll(files["extlinux/extlinux.conf"])
	if err != nil {
		t.Fatalf("reading extlinux.conf: %v", err)
	}
	if !strings.Contains(string(extlinuxConf), "console=ttyS0,115200n8") {
		t.Errorf("extlinux.conf = %q, want it to default to console=ttyS0,115200n8 when ConsoleBaud is unset", extlinuxConf)
	}
}

func TestBootFilesHonorsConsoleBaudOverride(t *testing.T) {
	b := cubiea5e.New()
	art := resolveFakeArtifacts(t, b)
	art.Initramfs = strings.NewReader("fake initramfs bytes")

	files, err := b.BootFiles(boards.BuildConfig{ConsoleBaud: 1500000}, art)
	if err != nil {
		t.Fatalf("BootFiles: %v", err)
	}
	extlinuxConf, err := io.ReadAll(files["extlinux/extlinux.conf"])
	if err != nil {
		t.Fatalf("reading extlinux.conf: %v", err)
	}
	if !strings.Contains(string(extlinuxConf), "console=ttyS0,1500000n8") {
		t.Errorf("extlinux.conf = %q, want it to contain console=ttyS0,1500000n8 when ConsoleBaud=1500000", extlinuxConf)
	}
	if strings.Contains(string(extlinuxConf), "115200") {
		t.Errorf("extlinux.conf = %q, want the default 115200 rate gone once overridden", extlinuxConf)
	}
}

func TestConsoleBaudSupportIsSupported(t *testing.T) {
	if got := cubiea5e.New().ConsoleBaudSupport(); !got.Supported {
		t.Errorf("ConsoleBaudSupport() = %+v, want Supported: true (extlinux.conf's console= rate is board-rendered)", got)
	}
}

func TestRawWritesOffsetAndContent(t *testing.T) {
	b := cubiea5e.New()
	art := resolveFakeArtifacts(t, b)

	writes := b.RawWrites(art)
	if len(writes) != 1 {
		t.Fatalf("RawWrites() = %d writes, want 1 (the sunxi boot chain is a single SPL+FIT write, unlike the Rockchip boards' pair)", len(writes))
	}

	w := writes[0]
	if w.OffsetBytes != 8192 {
		t.Errorf("RawWrites()[0].OffsetBytes = %d, want 8192 (8KiB, sector 16)", w.OffsetBytes)
	}

	data, err := io.ReadAll(w.Content)
	if err != nil {
		t.Fatalf("reading RawWrite content: %v", err)
	}
	if string(data) != "fake u-boot-sunxi-with-spl.bin" {
		t.Errorf("u-boot-sunxi-with-spl.bin content = %q, want the resolved artifact's content", data)
	}
}

func TestRawWritesPanicsWhenUbootTooBigForTheGap(t *testing.T) {
	dir := t.TempDir()
	b := cubiea5e.New()
	for _, ref := range b.Artifacts() {
		content := []byte("fake " + ref.Name)
		if ref.Name == "u-boot-sunxi-with-spl.bin" {
			// 16MiB starts at byte 16777216; u-boot-sunxi-with-spl.bin is
			// written at 8192, so anything over 16777216-8192 bytes
			// overruns it.
			content = make([]byte, 16*1024*1024-8192+1)
		}
		if err := os.WriteFile(filepath.Join(dir, ref.Name), content, 0o644); err != nil {
			t.Fatalf("seeding fake artifact %q: %v", ref.Name, err)
		}
	}

	art, err := boards.ResolveArtifacts(context.Background(), b.Name(), b.Artifacts(), dir, t.TempDir(), nil)
	if err != nil {
		t.Fatalf("ResolveArtifacts: %v", err)
	}

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("RawWrites() did not panic for an oversized u-boot-sunxi-with-spl.bin")
		}
		msg, ok := r.(string)
		if !ok || !strings.Contains(msg, "16MiB") {
			t.Errorf("panic value = %v, want a message mentioning the 16MiB boundary", r)
		}
	}()
	b.RawWrites(art)
}

func TestFirmwareFilesIsEmpty(t *testing.T) {
	if got := cubiea5e.New().FirmwareFiles(boards.Artifacts{}); len(got) != 0 {
		t.Errorf("FirmwareFiles() = %v, want empty: no runtime-loaded firmware in scope for this board", got)
	}
}

// TestUsbGadgetSupportIsRefusedUntilTheVariantDTBShips pins the corrected
// claim from bean gosd-3io0: this board cannot enumerate as a USB device at
// the pinned artifacts, because ehci0/ohci0 take the USB-C port's phy from
// the peripheral controller at probe. Refusing beats building an image that
// looks right and cannot work, and the reason has to name the fix.
func TestUsbGadgetSupportIsRefusedUntilTheVariantDTBShips(t *testing.T) {
	got := cubiea5e.New().UsbGadgetSupport()
	if got.Supported {
		t.Fatalf("UsbGadgetSupport() = %+v, want Supported: false while the pinned artifacts carry no gadget DTB", got)
	}
	if got.Reason == "" {
		t.Error("UsbGadgetSupport().Reason is empty; a refusal must tell the user why and what changes it")
	}
}

func TestEXT4SupportIsSupported(t *testing.T) {
	if got := cubiea5e.New().EXT4Support(); !got.Supported {
		t.Errorf("EXT4Support() = %+v, want Supported: true (stock kernel builds CONFIG_EXT4_FS=y)", got)
	}
}
