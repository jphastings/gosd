//go:build linux

package main

import (
	"fmt"
	"os"
	"syscall"
)

// mountVFAT mounts the FAT partition at device read-write on mountpoint,
// with the same options gosd-init uses for GOSD-DATA: nosuid/nodev, and
// vfat's "flush" so a journal-less FAT doesn't sit with dirty data in RAM
// on a board with no clean-shutdown story.
func mountVFAT(device, mountpoint string) error {
	if err := os.MkdirAll(mountpoint, 0o755); err != nil {
		return fmt.Errorf("creating mountpoint %s: %w", mountpoint, err)
	}
	if err := syscall.Mount(device, mountpoint, "vfat", syscall.MS_NOSUID|syscall.MS_NODEV, "flush"); err != nil {
		return fmt.Errorf("mount(%s, %s): %w", device, mountpoint, err)
	}
	return nil
}

// unmountVFAT releases the filesystem mounted at mountpoint so its block
// device can be handed to gadget.MassStorage exclusively — expose or mount,
// never both.
func unmountVFAT(mountpoint string) error {
	if err := syscall.Unmount(mountpoint, 0); err != nil {
		return fmt.Errorf("unmounting %s: %w", mountpoint, err)
	}
	return nil
}
