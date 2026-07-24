package qemuvirt_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jphastings/gosd/internal/boards"
	"github.com/jphastings/gosd/internal/boards/qemuvirt"
)

// resolveFakeArtifacts seeds a temp --artifacts-dir with a fake file for
// every artifact the board asks for, then resolves it - exercising the same
// path gosd's integration test uses, without needing a real compiled
// kernel.
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
	if got := qemuvirt.New().Name(); got != "qemu-virt" {
		t.Errorf("Name() = %q, want qemu-virt", got)
	}
}

func TestArtifactsIsKernelOnlyWithNoAutomaticFetchSource(t *testing.T) {
	refs := qemuvirt.New().Artifacts()

	if len(refs) != 1 || refs[0].Name != "Image" {
		t.Fatalf("Artifacts() = %+v, want exactly one ref named Image", refs)
	}
	if refs[0].URL != "" {
		t.Errorf("Image ArtifactRef has URL %q; it has no automatic fetch source yet and must come from --artifacts-dir only", refs[0].URL)
	}
}

func TestBootFilesRequiresAnInitramfs(t *testing.T) {
	b := qemuvirt.New()
	art := resolveFakeArtifacts(t, b)

	if _, err := b.BootFiles(boards.BuildConfig{}, art); err == nil {
		t.Fatal("BootFiles() without an initramfs succeeded, want an error")
	}
}

// TestBootFilesContentsHasNoBootloaderConfigFiles asserts the shape the bean
// locks in: just the kernel and initramfs, no config.txt/extlinux.conf -
// qemu is invoked with -kernel/-initrd directly, so there's nothing for a
// board-specific boot config file to do here.
func TestBootFilesContentsHasNoBootloaderConfigFiles(t *testing.T) {
	b := qemuvirt.New()
	art := resolveFakeArtifacts(t, b)
	art.Initramfs = strings.NewReader("fake initramfs bytes")

	files, err := b.BootFiles(boards.BuildConfig{}, art)
	if err != nil {
		t.Fatalf("BootFiles: %v", err)
	}

	if len(files) != 2 {
		t.Errorf("BootFiles() = %v, want exactly Image + initramfs.cpio.zst (no config.txt/extlinux.conf)", keys(files))
	}
	for _, want := range []string{"Image", "initramfs.cpio.zst"} {
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
}

func TestRawWritesIsEmpty(t *testing.T) {
	if got := qemuvirt.New().RawWrites(boards.Artifacts{}); len(got) != 0 {
		t.Errorf("RawWrites() = %v, want empty: qemu-virt has no bootloader in the unpartitioned gap", got)
	}
}

func TestUsbGadgetSupportIsUnsupported(t *testing.T) {
	got := qemuvirt.New().UsbGadgetSupport()
	if got.Supported {
		t.Fatal("UsbGadgetSupport().Supported = true, want false: the qemu-virt invocation attaches no USB controller device model")
	}
	if got.Reason == "" {
		t.Error("UsbGadgetSupport().Reason is empty, want an explanation")
	}
}

func TestFirmwareFilesIsEmpty(t *testing.T) {
	if got := qemuvirt.New().FirmwareFiles(boards.Artifacts{}); len(got) != 0 {
		t.Errorf("FirmwareFiles() = %v, want empty: virtio devices need no runtime-loaded firmware", got)
	}
}

func keys(m map[string]io.Reader) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
