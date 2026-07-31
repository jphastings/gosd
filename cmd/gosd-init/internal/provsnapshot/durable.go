package provsnapshot

import (
	"os"
	"path/filepath"
)

// WriteFileDurably writes data to path so that it survives a power cut
// happening immediately afterwards, following the four-step sequence
// docs/runtime.md's "Making a write durable" spells out and explains:
// write to a temporary name, fsync the file, rename it over the real name,
// fsync the file again (the rename leaves the new directory entry with a
// zero start cluster until it does), then fsync the containing directory.
//
// Both filesystems gosd-init writes to — GOSD-DATA and GOSD-BOOT — are FAT,
// which has no journal and holds the rename's directory blocks dirty for
// the kernel's full writeback expiry (~30s) without those last two syncs.
// A gosd-init write that a reboot or power cut can silently undo is worse
// than no write at all, since it's exactly a card being flashed or
// unplugged that this package's snapshot exists to survive.
func WriteFileDurably(path string, data []byte) error {
	tmp := path + ".tmp"
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
