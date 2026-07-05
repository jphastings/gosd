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

func TestArtifactsIncludesKernelAndManifestFiles(t *testing.T) {
	refs := pizero2w.New().Artifacts()

	names := make(map[string]boards.ArtifactRef, len(refs))
	for _, r := range refs {
		names[r.Name] = r
	}

	for _, want := range []string{
		"kernel8.img", "bootcode.bin", "start.elf", "fixup.dat",
		"brcmfmac43436-sdio.bin", "brcmfmac43436-sdio.clm_blob", "brcmfmac43436-sdio.txt",
		"brcmfmac43436s-sdio.bin", "brcmfmac43436s-sdio.txt",
	} {
		if _, ok := names[want]; !ok {
			t.Errorf("Artifacts() is missing %q", want)
		}
	}

	if kernel := names["kernel8.img"]; kernel.URL != "" {
		t.Errorf("kernel8.img has URL %q; it has no automatic fetch source yet and must come from --artifacts-dir only", kernel.URL)
	}

	for name, ref := range names {
		if name == "kernel8.img" {
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
		"kernel8.img", "bootcode.bin", "start.elf", "fixup.dat",
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
