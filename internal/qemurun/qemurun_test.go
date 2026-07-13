package qemurun_test

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jphastings/gosd/internal/image"
	"github.com/jphastings/gosd/internal/qemurun"
)

func TestArgsUsesDefaultsWhenPortAndMemoryAreZero(t *testing.T) {
	args := qemurun.Args("/work", "/img.img", qemurun.Options{})

	assertFlag(t, args, "-m", "512")
	assertFlag(t, args, "-kernel", filepath.Join("/work", "Image"))
	assertFlag(t, args, "-initrd", filepath.Join("/work", "initramfs.cpio.zst"))
	assertContains(t, args, "-drive", "if=none,file=/img.img,format=raw,id=hd0")
	assertContains(t, args, "-netdev", "user,id=n0,hostfwd=tcp::8080-:80")
}

func TestArgsHonorsPortMemoryAndExtraArgs(t *testing.T) {
	args := qemurun.Args("/work", "/img.img", qemurun.Options{
		Port:      9999,
		MemoryMiB: 1024,
		ExtraArgs: []string{"-smp", "2"},
	})

	assertFlag(t, args, "-m", "1024")
	assertContains(t, args, "-netdev", "user,id=n0,hostfwd=tcp::9999-:80")

	if got := args[len(args)-2:]; got[0] != "-smp" || got[1] != "2" {
		t.Errorf("ExtraArgs weren't appended last: got %v", got)
	}
}

func TestArgsIsHeadlessByDefaultButAlwaysAttachesAVirtioGPU(t *testing.T) {
	args := qemurun.Args("/work", "/img.img", qemurun.Options{})

	if !contains(args, "-nographic") {
		t.Errorf("default args %v missing -nographic", args)
	}
	if !contains(args, "virtio-gpu-pci,romfile=") {
		t.Errorf("default args %v missing the virtio-gpu-pci device", args)
	}
}

func TestArgsDisplaySwapsNographicForSerialOnStdio(t *testing.T) {
	args := qemurun.Args("/work", "/img.img", qemurun.Options{Display: true})

	if contains(args, "-nographic") {
		t.Errorf("Display args %v still contain -nographic", args)
	}
	if contains(args, "-display") {
		t.Errorf("Display args %v force a -display backend instead of qemu's host default", args)
	}
	assertFlag(t, args, "-serial", "mon:stdio")
	if !contains(args, "virtio-gpu-pci,romfile=") {
		t.Errorf("Display args %v missing the virtio-gpu-pci device", args)
	}
}

func contains(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

func TestExtractBootFilesCopiesEveryBootPartitionFile(t *testing.T) {
	imgPath := filepath.Join(t.TempDir(), "qemu-virt.img")
	if err := image.Write(imgPath, image.Spec{
		BootFiles: map[string]io.Reader{
			"Image":              strings.NewReader("fake kernel bytes"),
			"initramfs.cpio.zst": strings.NewReader("fake initramfs bytes"),
			"gosd.toml":          strings.NewReader("hostname = \"test\"\n"),
		},
	}); err != nil {
		t.Fatalf("image.Write: %v", err)
	}

	destDir := t.TempDir()
	if err := qemurun.ExtractBootFiles(imgPath, destDir); err != nil {
		t.Fatalf("ExtractBootFiles: %v", err)
	}

	for name, want := range map[string]string{
		"Image":              "fake kernel bytes",
		"initramfs.cpio.zst": "fake initramfs bytes",
		"gosd.toml":          "hostname = \"test\"\n",
	} {
		got, err := os.ReadFile(filepath.Join(destDir, name))
		if err != nil {
			t.Errorf("reading extracted %s: %v", name, err)
			continue
		}
		if string(got) != want {
			t.Errorf("extracted %s = %q, want %q", name, got, want)
		}
	}
}

func TestRunFailsActionablyWhenImagePathIsMissing(t *testing.T) {
	if _, err := exec.LookPath("qemu-system-aarch64"); err != nil {
		t.Skip("qemu-system-aarch64 not installed; only CheckAvailable's own error path is exercised elsewhere")
	}

	err := qemurun.Run(context.Background(), qemurun.Options{ImagePath: ""})
	if err == nil {
		t.Fatal("Run with an empty ImagePath succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "ImagePath is required") {
		t.Errorf("error = %q, want it to mention ImagePath is required", err.Error())
	}
}

func TestRunOnANonQemuVirtImageFailsActionably(t *testing.T) {
	if _, err := exec.LookPath("qemu-system-aarch64"); err != nil {
		t.Skip("qemu-system-aarch64 not installed")
	}

	imgPath := filepath.Join(t.TempDir(), "not-qemu-virt.img")
	if err := image.Write(imgPath, image.Spec{
		BootFiles: map[string]io.Reader{
			"gosd.toml": strings.NewReader("hostname = \"test\"\n"),
		},
	}); err != nil {
		t.Fatalf("image.Write: %v", err)
	}

	err := qemurun.Run(context.Background(), qemurun.Options{ImagePath: imgPath})
	if err == nil {
		t.Fatal("Run against an image with no Image/initramfs succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "--board=qemu-virt") {
		t.Errorf("error = %q, want it to suggest --board=qemu-virt", err.Error())
	}
}

func TestCheckAvailableErrorNamesAnInstallCommand(t *testing.T) {
	if _, err := exec.LookPath("qemu-system-aarch64"); err == nil {
		t.Skip("qemu-system-aarch64 is installed on this host; nothing to assert about the missing-binary path")
	}

	err := qemurun.CheckAvailable()
	if err == nil {
		t.Fatal("CheckAvailable succeeded despite qemu-system-aarch64 not being on PATH")
	}
	if !strings.Contains(err.Error(), "qemu-system-aarch64") {
		t.Errorf("error = %q, want it to name the missing binary", err.Error())
	}
}

func assertFlag(t *testing.T, args []string, flag, want string) {
	t.Helper()
	for i, a := range args {
		if a == flag {
			if i+1 >= len(args) {
				t.Fatalf("flag %s has no value", flag)
			}
			if args[i+1] != want {
				t.Errorf("%s = %q, want %q", flag, args[i+1], want)
			}
			return
		}
	}
	t.Errorf("args %v missing flag %s", args, flag)
}

func assertContains(t *testing.T, args []string, flag, want string) {
	t.Helper()
	assertFlag(t, args, flag, want)
}
