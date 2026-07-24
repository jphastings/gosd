package rock4se_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jphastings/gosd/internal/boards"
	"github.com/jphastings/gosd/internal/boards/rock4se"
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
	if got := rock4se.New().Name(); got != "rock-4se" {
		t.Errorf("Name() = %q, want rock-4se", got)
	}
}

func TestArtifactsHasNoAutomaticFetchSource(t *testing.T) {
	refs := rock4se.New().Artifacts()

	names := make(map[string]boards.ArtifactRef, len(refs))
	for _, r := range refs {
		names[r.Name] = r
	}

	for _, want := range []string{"idbloader.img", "u-boot.itb", "Image", "rk3399-rock-4se.dtb"} {
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
	b := rock4se.New()
	art := resolveFakeArtifacts(t, b)

	if _, err := b.BootFiles(boards.BuildConfig{}, art); err == nil {
		t.Fatal("BootFiles() without an initramfs succeeded, want an error")
	}
}

func TestBootFilesContents(t *testing.T) {
	b := rock4se.New()
	art := resolveFakeArtifacts(t, b)
	art.Initramfs = strings.NewReader("fake initramfs bytes")

	files, err := b.BootFiles(boards.BuildConfig{}, art)
	if err != nil {
		t.Fatalf("BootFiles: %v", err)
	}

	for _, want := range []string{"Image", "rk3399-rock-4se.dtb", "initramfs.cpio.zst", "extlinux/extlinux.conf"} {
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
	if !strings.Contains(string(extlinuxConf), "gosd.board=rock-4se") {
		t.Errorf("extlinux.conf = %q, want it to contain gosd.board=rock-4se", extlinuxConf)
	}
}

func TestBootFilesIgnoresUsbGadget(t *testing.T) {
	b := rock4se.New()
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

func TestRawWritesOffsetsAndContent(t *testing.T) {
	b := rock4se.New()
	art := resolveFakeArtifacts(t, b)

	writes := b.RawWrites(art)
	if len(writes) != 2 {
		t.Fatalf("RawWrites() = %d writes, want 2", len(writes))
	}

	byOffset := make(map[int64][]byte, len(writes))
	for _, w := range writes {
		data, err := io.ReadAll(w.Content)
		if err != nil {
			t.Fatalf("reading RawWrite content at offset %d: %v", w.OffsetBytes, err)
		}
		byOffset[w.OffsetBytes] = data
	}

	idbloader, ok := byOffset[32768]
	if !ok {
		t.Fatal("RawWrites() has no write at offset 32768 (idbloader.img)")
	}
	if string(idbloader) != "fake idbloader.img" {
		t.Errorf("idbloader.img content = %q, want the resolved artifact's content", idbloader)
	}

	uboot, ok := byOffset[8388608]
	if !ok {
		t.Fatal("RawWrites() has no write at offset 8388608 (u-boot.itb)")
	}
	if string(uboot) != "fake u-boot.itb" {
		t.Errorf("u-boot.itb content = %q, want the resolved artifact's content", uboot)
	}
}

func TestRawWritesPanicsWhenUbootTooBigForTheGap(t *testing.T) {
	dir := t.TempDir()
	b := rock4se.New()
	for _, ref := range b.Artifacts() {
		content := []byte("fake " + ref.Name)
		if ref.Name == "u-boot.itb" {
			// 16MiB starts at byte 16777216; u-boot.itb is written at
			// 8388608, so anything over 8388608 bytes overruns it.
			content = make([]byte, 8388608+1)
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
			t.Fatal("RawWrites() did not panic for an oversized u-boot.itb")
		}
		msg, ok := r.(string)
		if !ok || !strings.Contains(msg, "16MiB") {
			t.Errorf("panic value = %v, want a message mentioning the 16MiB boundary", r)
		}
	}()
	b.RawWrites(art)
}

func TestFirmwareFilesIsEmpty(t *testing.T) {
	if got := rock4se.New().FirmwareFiles(boards.Artifacts{}); len(got) != 0 {
		t.Errorf("FirmwareFiles() = %v, want empty: no runtime-loaded firmware on this board in v0.1", got)
	}
}

func TestUsbGadgetSupportIsSupported(t *testing.T) {
	if got := rock4se.New().UsbGadgetSupport(); !got.Supported {
		t.Errorf("UsbGadgetSupport() = %+v, want Supported: true (dr_mode DTS patch bakes gadget mode into the device tree)", got)
	}
}
