package diskfmt

import (
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/diskfs/go-diskfs"
	"github.com/diskfs/go-diskfs/filesystem"
)

// backingFile stands in for a real block device: a sparse regular file of a
// realistic size. openDisk sizes both the same way — lseek to the end — so
// everything from the open onward is exactly the path a real device takes.
func backingFile(t *testing.T, sizeBytes int64) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "device.img")
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

func TestInspectBlankDevice(t *testing.T) {
	got, err := Inspect(backingFile(t, 8*1024*1024))
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if got.FS != "" || !got.Blank {
		t.Errorf("Inspect of a zeroed device = %+v, want no filesystem and Blank:true", got)
	}
}

func TestInspectForeignContentIsNotBlank(t *testing.T) {
	path := backingFile(t, 8*1024*1024)
	// A stray non-zero byte in the leading region stands in for a foreign
	// partition table or filesystem: readable as neither our FAT nor blank.
	scribble(t, path, 0, []byte{0xEB, 0x00, 0x55, 0xAA})

	got, err := Inspect(path)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if got.FS != "" || got.Blank {
		t.Errorf("Inspect of foreign content = %+v, want no filesystem and Blank:false", got)
	}
}

// TestInspectNamesUnreadableExFATWithoutClaimingItIsMountable covers a device
// whose boot sector announces exFAT but whose geometry does not parse: it is
// named, so a refusal can be specific, but it is neither readable nor blank, so
// it is still refused without an explicit destructive opt-in.
func TestInspectNamesUnreadableExFATWithoutClaimingItIsMountable(t *testing.T) {
	path := backingFile(t, 8*1024*1024)
	scribble(t, path, 0, append([]byte{0xEB, 0x76, 0x90}, []byte("EXFAT   ")...))

	got, err := Inspect(path)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if got.OtherFS != "exFAT" || got.FS != "" || got.Blank {
		t.Errorf("Inspect of a broken exFAT device = %+v, want {OtherFS:exFAT} and nothing else", got)
	}
}

func TestInspectReportsFATLabel(t *testing.T) {
	path := backingFile(t, 64*1024*1024)
	if err := FormatFAT32(path, "APPDATA"); err != nil {
		t.Fatalf("FormatFAT32: %v", err)
	}

	got, err := Inspect(path)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if got.FS != FAT32 || got.Label != "APPDATA" {
		t.Errorf("Inspect of formatted device = %+v, want {FS:fat32 Label:APPDATA}", got)
	}
}

// TestFormatFAT32SpansADevicePast4GiB pins the gosd-fjio fix. go-diskfs's own
// block-device sizing reads BLKGETSIZE64's u64 into a Go int — 4 bytes on
// 32-bit ARM — so a >= 4GiB device used to be laid out for a truncated
// fraction of itself. Sizing now comes from lseek, shared by files and
// devices, so the resulting geometry must span the whole device.
func TestFormatFAT32SpansADevicePast4GiB(t *testing.T) {
	const size = 5 << 30 // past every 32-bit truncation boundary
	path := backingFile(t, size)

	if err := FormatFAT32(path, "BIGDATA"); err != nil {
		t.Fatalf("FormatFAT32: %v", err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("reopening formatted device: %v", err)
	}
	defer func() { _ = f.Close() }()
	sector := make([]byte, 512)
	if _, err := io.ReadFull(f, sector); err != nil {
		t.Fatalf("reading boot sector back: %v", err)
	}
	// TotSec32, the FAT32 total-sector count, at offset 32 of the boot sector.
	if got, want := binary.LittleEndian.Uint32(sector[32:36]), uint32(size/512); got != want {
		t.Errorf("boot sector total sectors = %d (%.1f GiB), want %d (the whole device)",
			got, float64(got)*512/(1<<30), want)
	}

	got, err := Inspect(path)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if got.FS != FAT32 || got.Label != "BIGDATA" {
		t.Errorf("Inspect of formatted device = %+v, want {FS:fat32 Label:BIGDATA}", got)
	}
}

// scribble writes raw bytes at offset into the file at path, without disturbing
// the rest of it.
func scribble(t *testing.T, path string, offset int64, data []byte) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("opening to scribble: %v", err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteAt(data, offset); err != nil {
		t.Fatalf("scribbling: %v", err)
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
