// Command emmcstorage is a minimal example demonstrating the emmc package: on a
// board with onboard eMMC (Radxa Zero 3E, NanoPi Zero2), it formats the eMMC
// on first boot and mounts it read-write at /storage on every boot after
// that, then writes a small file there and reads it back to demonstrate
// persistence. On a board with no onboard eMMC (e.g. a Pi board), it logs
// that plainly and exits cleanly rather than treating the absence as a
// failure — see docs/runtime.md's "Onboard eMMC" section.
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jphastings/gosd/emmc"
)

const (
	label      = "APPDATA"
	mountpoint = "/storage"
)

func main() {
	res := <-emmc.FormatAndMount(label, mountpoint, false)
	if res.Err != nil {
		if errors.Is(res.Err, emmc.ErrNoEMMC) {
			fmt.Println("gosd emmc: no onboard eMMC on this board - nothing to do")
			return
		}
		fmt.Fprintf(os.Stderr, "gosd emmc: %v\n", res.Err)
		os.Exit(1)
	}

	fmt.Printf("gosd emmc: %s ready at %s (device %s)\n", label, res.MountPoint, res.BlockDevice)

	if err := writeAndReadBack(); err != nil {
		fmt.Fprintf(os.Stderr, "gosd emmc: %v\n", err)
		os.Exit(1)
	}
}

// writeAndReadBack demonstrates the eMMC's persistence: it durably writes a
// small file, then reads it straight back to confirm it landed.
func writeAndReadBack() error {
	path := filepath.Join(mountpoint, "hello-emmc.txt")

	if err := writeFileDurably(path, []byte("hello from gosd emmc example\n")); err != nil {
		return fmt.Errorf("writing %s failed: %w", path, err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading back %s failed: %w", path, err)
	}
	fmt.Printf("gosd emmc: wrote and read back %s: %q\n", path, string(data))
	return nil
}

// writeFileDurably writes data to path so that a power cut leaves either the
// old contents or the new, never a torn mix, and so that the new contents are
// on the device by the time it returns: write a temp file, fsync it, rename it
// over the real name, then fsync the renamed file and its directory. Even
// ext4's journal (the eMMC's default filesystem, see the emmc package doc)
// only buys metadata crash-consistency, not durability for this file's own
// data, so the same pattern applies regardless of filesystem — see
// docs/runtime.md's "Making a write durable".
func writeFileDurably(path string, data []byte) error {
	tmp := path + ".tmp"

	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = f.Close()
		return err
	}
	// A FAT rename only dirties directory blocks, which otherwise wait for
	// writeback expiry (~30s). Syncing the still-open file writes its new
	// directory entry with the real start cluster and size; syncing the
	// directory writes the entry the rename added and the one it removed.
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return syncDir(filepath.Dir(path))
}

// syncDir fsyncs a directory itself, so directory entries added or removed in
// it reach the device rather than waiting for writeback.
func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	if err := d.Sync(); err != nil {
		_ = d.Close()
		return err
	}
	return d.Close()
}
