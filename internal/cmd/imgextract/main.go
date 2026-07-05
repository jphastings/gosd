// Command imgextract copies every file at the root of a gosd .img's
// GOSD-BOOT FAT partition out to a destination directory, without root and
// without mtools: it opens the image read-only via github.com/diskfs/go-
// diskfs and reads the FAT32 filesystem directly.
//
// It exists for scripts/qemu-run.sh, which needs the kernel Image and
// initramfs.cpio.zst a qemu-virt image carries on its boot partition before
// it can hand them to `qemu-system-aarch64 -kernel/-initrd` — qemu has no
// way to boot straight from the partition image itself the way real
// hardware's bootloader does.
//
// Usage: go run ./internal/cmd/imgextract <image.img> <dest-dir>
package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	diskfs "github.com/diskfs/go-diskfs"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "imgextract: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: imgextract <image.img> <dest-dir>")
	}
	imgPath, destDir := args[0], args[1]

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
			continue // gosd's boot partitions are flat; nested dirs (if any) aren't what qemu-run.sh needs
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
