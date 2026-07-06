// Package qemurun boots a gosd --board=qemu-virt image under
// qemu-system-aarch64: extracting the kernel and initramfs from the
// image's FAT boot partition (qemu has no bootloader of its own to read
// them off the partition the way real hardware does), then exec'ing the
// qemu invocation validated by gosd-5wm0 and proven end-to-end by
// gosd-27lz.
//
// It is the one place that invocation lives: both `gosd run` (cmd/gosd)
// and scripts/qemu-run.sh (via internal/cmd/qemuboot) call into this
// package rather than each keeping their own copy of the qemu flags.
package qemurun

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"time"

	diskfs "github.com/diskfs/go-diskfs"
)

// DefaultPort is the host port forwarded to the guest's HTTP port 80 when
// Options.Port is zero.
const DefaultPort = 8080

// DefaultMemoryMiB is the guest RAM size when Options.MemoryMiB is zero,
// matching the qemu-virt invocation validated by gosd-5wm0.
const DefaultMemoryMiB = 512

// binary is the qemu binary this package invokes.
const binary = "qemu-system-aarch64"

// requiredBootFiles are the boot-partition files Run's -kernel/-initrd
// point at. Their absence after extraction means imgPath isn't a
// --board=qemu-virt image.
var requiredBootFiles = []string{"Image", "initramfs.cpio.zst"}

// Options configures a qemu-virt boot.
type Options struct {
	// ImagePath is the built --board=qemu-virt .img to boot.
	ImagePath string
	// Port is the host port forwarded to the guest's HTTP port 80. Zero
	// means DefaultPort.
	Port int
	// MemoryMiB is the guest's RAM size in MiB. Zero means
	// DefaultMemoryMiB.
	MemoryMiB int
	// ExtraArgs are appended verbatim after every other
	// qemu-system-aarch64 argument - an escape hatch for options this
	// package doesn't expose directly.
	ExtraArgs []string

	Stdin          io.Reader
	Stdout, Stderr io.Writer
}

// CheckAvailable returns an actionable error if qemu-system-aarch64 isn't
// on PATH. It has no minimum version check: every qemu-system-aarch64
// tested so far (7.2.x in CI's apt package, current Homebrew on macOS)
// supports the invocation Args builds, so there's nothing yet worth
// gating on.
func CheckAvailable() error {
	if _, err := exec.LookPath(binary); err != nil {
		return fmt.Errorf("%s not found on PATH; install it first: %s", binary, installHint())
	}
	return nil
}

func installHint() string {
	switch runtime.GOOS {
	case "darwin":
		return "brew install qemu"
	case "linux":
		return "apt-get install qemu-system-arm (Debian/Ubuntu) or your distribution's qemu-system-aarch64 package"
	default:
		return "see https://www.qemu.org/download/ for " + runtime.GOOS
	}
}

// ExtractBootFiles copies every file at the root of imgPath's GOSD-BOOT
// FAT partition into destDir, without root and without mtools: it opens
// the image read-only via go-diskfs and reads the FAT32 filesystem
// directly.
func ExtractBootFiles(imgPath, destDir string) error {
	d, err := diskfs.Open(imgPath, diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		return fmt.Errorf("opening %s: %w", imgPath, err)
	}
	defer func() { _ = d.Close() }()

	bootFS, err := d.GetFilesystem(1)
	if err != nil {
		return fmt.Errorf("reading the boot partition (partition 1) of %s: %w", imgPath, err)
	}

	entries, err := bootFS.ReadDir(".")
	if err != nil {
		return fmt.Errorf("listing the boot partition root of %s: %w", imgPath, err)
	}

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", destDir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue // gosd's boot partitions are flat
		}
		if err := extractFile(bootFS, entry, destDir); err != nil {
			return err
		}
	}

	return nil
}

func extractFile(bootFS fs.ReadFileFS, entry fs.DirEntry, destDir string) error {
	data, err := bootFS.ReadFile(entry.Name())
	if err != nil {
		return fmt.Errorf("reading %s from the boot partition: %w", entry.Name(), err)
	}
	destPath := filepath.Join(destDir, entry.Name())
	if err := os.WriteFile(destPath, data, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", destPath, err)
	}
	return nil
}

// Args builds the qemu-system-aarch64 argument list for booting the
// kernel and initramfs already extracted into workDir, with imgPath
// attached as the virtio-blk disk: -M virt -cpu cortex-a53, virtio-blk and
// virtio-net over PCI with romfile= (avoids qemu refusing to start when
// PXE option ROMs aren't shipped), serial on stdio, hostfwd for the
// guest's HTTP port. This is the exact invocation validated by gosd-5wm0.
func Args(workDir, imgPath string, opts Options) []string {
	port := opts.Port
	if port == 0 {
		port = DefaultPort
	}
	mem := opts.MemoryMiB
	if mem == 0 {
		mem = DefaultMemoryMiB
	}

	args := []string{
		"-M", "virt", "-cpu", "cortex-a53", "-m", strconv.Itoa(mem),
		"-nographic",
		"-kernel", filepath.Join(workDir, "Image"),
		"-initrd", filepath.Join(workDir, "initramfs.cpio.zst"),
		"-append", "console=ttyAMA0 gosd.board=qemu-virt",
		"-drive", "if=none,file=" + imgPath + ",format=raw,id=hd0",
		"-device", "virtio-blk-pci,drive=hd0,romfile=",
		"-netdev", fmt.Sprintf("user,id=n0,hostfwd=tcp::%d-:80", port),
		"-device", "virtio-net-pci,netdev=n0,romfile=",
	}
	return append(args, opts.ExtraArgs...)
}

// Run extracts opts.ImagePath's boot files into a temp directory and boots
// it under qemu-system-aarch64, with stdio wired to
// opts.Stdin/Stdout/Stderr so the guest's serial console behaves like a
// real serial cable.
//
// Run returns when qemu exits on its own, or when ctx is cancelled: on
// cancellation qemu is sent SIGTERM first (its monitor has no other way to
// shut down cleanly from outside) and killed outright if it hasn't exited
// within a few seconds. A ctx cancellation is reported as a nil error -
// it's how callers (gosd run's Ctrl-C handling) ask for a clean stop, not
// a failure.
func Run(ctx context.Context, opts Options) error {
	if err := CheckAvailable(); err != nil {
		return err
	}
	if opts.ImagePath == "" {
		return fmt.Errorf("qemurun: ImagePath is required")
	}

	workDir, err := os.MkdirTemp("", "gosd-qemurun-")
	if err != nil {
		return fmt.Errorf("creating a temp directory for the extracted boot files failed: %w", err)
	}
	defer func() { _ = os.RemoveAll(workDir) }()

	if err := ExtractBootFiles(opts.ImagePath, workDir); err != nil {
		return err
	}
	for _, name := range requiredBootFiles {
		if _, err := os.Stat(filepath.Join(workDir, name)); err != nil {
			return fmt.Errorf("%s's boot partition has no %s; is this a --board=qemu-virt image?", opts.ImagePath, name)
		}
	}

	absImg, err := filepath.Abs(opts.ImagePath)
	if err != nil {
		return fmt.Errorf("resolving %s: %w", opts.ImagePath, err)
	}

	cmd := exec.CommandContext(ctx, binary, Args(workDir, absImg, opts)...)
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = 5 * time.Second
	cmd.Stdin = opts.Stdin
	cmd.Stdout = opts.Stdout
	cmd.Stderr = opts.Stderr

	runErr := cmd.Run()
	if ctx.Err() != nil {
		return nil
	}
	if runErr != nil {
		return fmt.Errorf("qemu-system-aarch64 exited unexpectedly: %w", runErr)
	}
	return nil
}
