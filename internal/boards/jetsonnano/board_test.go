package jetsonnano_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jphastings/gosd/internal/boards"
	"github.com/jphastings/gosd/internal/boards/jetsonnano"
)

// resolveFakeArtifacts seeds a temp --artifacts-dir with a fake file for
// every artifact the board asks for, then resolves it, so nothing is fetched.
func resolveFakeArtifacts(t *testing.T) boards.Artifacts {
	t.Helper()
	b := jetsonnano.New()

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

func readAll(t *testing.T, r io.Reader) string {
	t.Helper()
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	return string(data)
}

func TestArtifactsPinOnlyTheFirmware(t *testing.T) {
	for _, ref := range jetsonnano.New().Artifacts() {
		pinned := ref.URL != "" && ref.SHA256 != ""
		if wantPinned := ref.Name == "xusb.bin"; pinned != wantPinned {
			t.Errorf("artifact %q pinned = %v, want %v: only the linux-firmware blob is fetched by URL; the kernel and DTB come from gosd build-kernel", ref.Name, pinned, wantPinned)
		}
	}
}

func TestBootFilesContents(t *testing.T) {
	art := resolveFakeArtifacts(t)
	art.Initramfs = strings.NewReader("fake initramfs bytes")

	files, err := jetsonnano.New().BootFiles(boards.BuildConfig{}, art)
	if err != nil {
		t.Fatalf("BootFiles: %v", err)
	}

	want := map[string]string{
		"Image":                   "fake Image",
		"tegra210-p3450-0000.dtb": "fake tegra210-p3450-0000.dtb",
		"initramfs.cpio.zst":      "fake initramfs bytes",
	}
	for name, content := range want {
		r, ok := files[name]
		if !ok {
			t.Errorf("BootFiles() is missing %q", name)
			continue
		}
		if got := readAll(t, r); got != content {
			t.Errorf("%s content = %q, want %q", name, got, content)
		}
	}

	extlinux := readAll(t, files["extlinux/extlinux.conf"])
	for _, want := range []string{"fdt /tegra210-p3450-0000.dtb", "console=ttyS0,115200n8", "gosd.board=jetson-nano"} {
		if !strings.Contains(extlinux, want) {
			t.Errorf("extlinux.conf = %q, want it to contain %q", extlinux, want)
		}
	}
}

func TestBootFilesRequiresAnInitramfs(t *testing.T) {
	if _, err := jetsonnano.New().BootFiles(boards.BuildConfig{}, resolveFakeArtifacts(t)); err == nil {
		t.Fatal("BootFiles() without an initramfs succeeded, want an error")
	}
}

func TestBootFilesHonorsConsoleBaudOverride(t *testing.T) {
	art := resolveFakeArtifacts(t)
	art.Initramfs = strings.NewReader("fake initramfs bytes")

	files, err := jetsonnano.New().BootFiles(boards.BuildConfig{ConsoleBaud: 9600}, art)
	if err != nil {
		t.Fatalf("BootFiles: %v", err)
	}
	if extlinux := readAll(t, files["extlinux/extlinux.conf"]); !strings.Contains(extlinux, "console=ttyS0,9600n8") {
		t.Errorf("extlinux.conf = %q, want console=ttyS0,9600n8", extlinux)
	}
}

func TestNoRawWrites(t *testing.T) {
	if got := jetsonnano.New().RawWrites(resolveFakeArtifacts(t)); len(got) != 0 {
		t.Errorf("RawWrites() = %d writes, want none: the bootloader lives in the module's QSPI", len(got))
	}
}

func TestFirmwareFilesShipsXUSBWhereXHCILooksForIt(t *testing.T) {
	files := jetsonnano.New().FirmwareFiles(resolveFakeArtifacts(t))

	if len(files) != 1 {
		t.Fatalf("FirmwareFiles() = %d files, want exactly the XUSB blob", len(files))
	}
	r, ok := files["nvidia/tegra210/xusb.bin"]
	if !ok {
		t.Fatalf("FirmwareFiles() has no nvidia/tegra210/xusb.bin, the path xhci-tegra requests")
	}
	if got := readAll(t, r); got != "fake xusb.bin" {
		t.Errorf("xusb.bin content = %q, want the resolved artifact's", got)
	}
}

func TestDeclaresTheFourteenMiBInitramfsCeiling(t *testing.T) {
	limiter, ok := jetsonnano.New().(boards.InitramfsLimiter)
	if !ok {
		t.Fatal("jetson-nano does not implement boards.InitramfsLimiter")
	}
	if got := limiter.MaxInitramfsBytes(); got != 14_680_064 {
		t.Fatalf("MaxInitramfsBytes() = %d, want 14,680,064", got)
	}
}
