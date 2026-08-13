// Package durable holds the one file-write gosd-init uses whenever a write
// has to survive the power being cut immediately afterwards: the boot
// counter on the data partition, a value written into the card's config
// tree, a crash-report cleanup. Every caller shares it so there is exactly
// one implementation of the ordering, and one place to reason about it.
package durable

import (
	"os"
	"path/filepath"
)

// WriteFile writes data to path so that it survives a power cut happening
// immediately afterwards, following the four-step sequence docs/runtime.md's
// "Making a write durable" spells out and explains: write to a temporary
// name, fsync the file, rename it over the real name, fsync the file again
// (the rename leaves the new directory entry with a zero start cluster
// until it does), then fsync the containing directory.
//
// The boot partition gosd-init writes to is FAT, which has no journal and
// holds the rename's directory blocks dirty for the kernel's full writeback
// expiry (~30s) without those last two syncs; the data partition is FAT
// too unless the image was built with --data-filesystem=ext4, whose journal
// covers the metadata but still never promises the file's own data. A
// gosd-init write that a reboot or power cut can silently undo is worse
// than no write at all: it is exactly a card being flashed or unplugged
// that these writes exist to survive.
func WriteFile(path string, data []byte) error {
	tmp := TempName(path)
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}

	if err := writeAndRename(f, tmp, path, data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	if err := dir.Sync(); err != nil {
		_ = dir.Close()
		return err
	}
	return dir.Close()
}

// TempName is the name WriteFile writes through before renaming: the
// file's own, with a leading period and a .tmp suffix. The period is what
// matters — a power cut between the write and the rename leaves this file
// behind, and inside the card's config tree a leftover named
// "hostname.tmp" would read as a setting called exactly that, where a
// dot-file is ignored by the device and refused by the build alike (see
// internal/configtree.IgnoredName). Exported so that invariant can be
// tested rather than assumed.
func TempName(path string) string {
	return filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".tmp")
}

// writeAndRename performs steps 1-3, leaving f open for its caller to close
// (step 3 needs the file's own descriptor after the rename).
func writeAndRename(f *os.File, tmp, path string, data []byte) error {
	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	return f.Sync()
}
