package emmcfmt

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/diskfs/go-diskfs"
	"github.com/diskfs/go-diskfs/filesystem"
)

// backingFile stands in for the eMMC block device: a sparse regular file of a
// realistic size. go-diskfs sizes a regular file from its Stat size and a real
// block device from ioctl(BLKGETSIZE64); the FAT32 formatting path downstream
// is identical, so this exercises everything except the ioctl itself (which
// needs real hardware/root and is a documented follow-up).
func backingFile(t *testing.T, sizeBytes int64) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "emmc.img")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("creating backing file: %v", err)
	}
	if err := f.Truncate(sizeBytes); err != nil {
		t.Fatalf("sizing backing file: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("closing backing file: %v", err)
	}
	return path
}

// TestFormatFAT32ProducesUsableFilesystem is the spike's headline proof: after
// FormatFAT32 the device carries a labelled FAT32 filesystem that a file
// survives a full write / reopen / read round-trip through — i.e. it is a real,
// mountable filesystem, formatted entirely in pure Go with no external mkfs.
func TestFormatFAT32ProducesUsableFilesystem(t *testing.T) {
	const label = "GOSD-EMMC"
	path := backingFile(t, 128*1024*1024)

	if err := FormatFAT32(path, label); err != nil {
		t.Fatalf("FormatFAT32: %v", err)
	}

	want := []byte("persisted across a reopen")
	writeThenClose(t, path, "/hello.txt", want)

	d, err := diskfs.Open(path, diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		t.Fatalf("reopening formatted device: %v", err)
	}
	defer func() { _ = d.Close() }()

	fs, err := d.GetFilesystem(0)
	if err != nil {
		t.Fatalf("reading filesystem back: %v", err)
	}
	if fs.Type() != filesystem.TypeFat32 {
		t.Errorf("filesystem type = %v, want FAT32", fs.Type())
	}
	if got := trimLabel(fs.Label()); got != label {
		t.Errorf("volume label = %q, want %q", got, label)
	}

	f, err := fs.OpenFile("/hello.txt", os.O_RDONLY)
	if err != nil {
		t.Fatalf("opening file back: %v", err)
	}
	got, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("reading file back: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("file contents = %q, want %q", got, want)
	}
}

// writeThenClose opens the just-formatted filesystem fresh, writes content to
// name, and fully closes everything — so the read-back in the test proves the
// bytes reached the filesystem, not a still-open buffer.
func writeThenClose(t *testing.T, devicePath, name string, content []byte) {
	t.Helper()
	d, err := diskfs.Open(devicePath, diskfs.WithOpenMode(diskfs.ReadWrite))
	if err != nil {
		t.Fatalf("opening device to write: %v", err)
	}
	fs, err := d.GetFilesystem(0)
	if err != nil {
		t.Fatalf("getting filesystem to write: %v", err)
	}
	f, err := fs.OpenFile(name, os.O_CREATE|os.O_RDWR)
	if err != nil {
		t.Fatalf("creating %s: %v", name, err)
	}
	if _, err := f.Write(content); err != nil {
		t.Fatalf("writing %s: %v", name, err)
	}
	if err := d.Close(); err != nil {
		t.Fatalf("closing device after write (flush) failed: %v", err)
	}
}

// trimLabel drops the trailing padding FAT stores volume labels with, so the
// comparison is against the meaningful characters only.
func trimLabel(label string) string {
	for len(label) > 0 && (label[len(label)-1] == ' ' || label[len(label)-1] == 0) {
		label = label[:len(label)-1]
	}
	return label
}
