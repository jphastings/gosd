package diskfmt

import (
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/diskfs/go-diskfs"
	"github.com/diskfs/go-diskfs/disk"
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

func TestRootFileRoundTripsWithoutMounting(t *testing.T) {
	const marker = "gosd-data-established"
	path := backingFile(t, 128*1024*1024)
	if err := FormatFAT32(path, "GOSD-DATA"); err != nil {
		t.Fatalf("FormatFAT32: %v", err)
	}

	// A freshly formatted volume carries nothing, which is what makes the
	// file's presence meaningful to whoever writes one.
	if found, err := RootFileExists(path, marker); err != nil || found {
		t.Fatalf("RootFileExists on a fresh filesystem = (%v, %v), want (false, nil)", found, err)
	}

	if err := CreateEmptyFile(path, marker); err != nil {
		t.Fatalf("CreateEmptyFile: %v", err)
	}
	if found, err := RootFileExists(path, marker); err != nil || !found {
		t.Fatalf("RootFileExists after creating it = (%v, %v), want (true, nil)", found, err)
	}
	if found, err := RootFileExists(path, "gosd-something-else"); err != nil || found {
		t.Errorf("RootFileExists for an absent name = (%v, %v), want (false, nil)", found, err)
	}
}

func TestRootFileExistsReportsAnUnreadableFilesystem(t *testing.T) {
	// Blank space is not a filesystem: "no" would be a lie, so it errors.
	if _, err := RootFileExists(backingFile(t, 8*1024*1024), "gosd-data-established"); err == nil {
		t.Error("RootFileExists on a blank device = nil error, want a failure to read it")
	}
}

// TestRootFileExistsSurvivesKernelDeletedEntries pins go-diskfs's handling of
// a directory shape gosd-zzdz found on a real qemu-virt data partition after
// a boot: examples/hello's write-temp/rename dance (docs/runtime.md) leaves
// the OLD file's directory slots — two LFN continuation entries plus their
// SFN — marked deleted the way the Linux kernel's vfat driver does it: the
// first byte of every slot in the group overwritten with 0xE5, including the
// LFN slots, in place, with no compaction. A raw capture from that
// investigation (offsets into the data partition):
//
//	0x11100060 LFN seq=e5 'mp'
//	0x11100080 LFN seq=e5 'hello-boots.t'
//	0x111000a0 SFN 'åELLO-~1TMP' attr=20   (0xE5 = deleted)
//	0x111000c0 LFN seq=41 'hello-boots'
//	0x111000e0 SFN 'HELLO-~1   ' attr=20
//
// gosd-zzdz's working theory was that this shape broke go-diskfs's FAT32
// reader outright ("invalid argument"). Reproducing it byte-for-byte (see
// deleteLFNGroup below, which patches exactly this pattern into a real
// go-diskfs-written directory) shows v1.9.3 parses it correctly:
// parseDirEntries checks a slot's delete marker before it looks at the LFN
// attribute byte, so a 0xE5'd LFN slot is skipped exactly like a 0xE5'd SFN
// slot, never mistaken for a malformed chain — go-diskfs's own Rename can't
// produce this shape to test against, since it always rewrites a directory's
// entries fresh rather than patching bytes in place, which is why the gap
// went unnoticed upstream.
//
// The literal "invalid argument" string is io/fs.ErrInvalid, which
// go-diskfs's path validation returns unconditionally for a rooted path like
// "/" regardless of directory content (see RootFileExists's own "." vs "/"
// note) — not something this entry shape triggers. No workaround lands here
// because there is nothing to work around; this test exists so a future
// go-diskfs bump can't silently regress the correct behaviour, and gosd-e721
// records the analysis for anyone tempted to send a fix upstream for a bug
// that turned out not to exist.
func TestRootFileExistsSurvivesKernelDeletedEntries(t *testing.T) {
	path := backingFile(t, 64*1024*1024)
	if err := FormatFAT32(path, "GOSD-DATA"); err != nil {
		t.Fatalf("FormatFAT32: %v", err)
	}

	// Two files back to back, the same order the durable-write dance leaves
	// them in: the temp file first (2 LFN slots + 1 SFN, "hello-boots.tmp"
	// needs 15 chars), then the file it gets renamed to (1 LFN slot + 1 SFN,
	// "hello-boots" needs 11).
	writeThenClose(t, path, "/hello-boots.tmp", []byte("1\n"))
	writeThenClose(t, path, "/hello-boots", []byte("2\n"))

	deleteLFNGroup(t, path)

	if found, err := RootFileExists(path, "hello-boots"); err != nil || !found {
		t.Fatalf("RootFileExists(hello-boots) = (%v, %v), want (true, nil)", found, err)
	}
	if found, err := RootFileExists(path, "hello-boots.tmp"); err != nil || found {
		t.Fatalf("RootFileExists(hello-boots.tmp) = (%v, %v), want (false, nil): it is marked deleted", found, err)
	}
}

// deleteLFNGroup locates the first "2 LFN slots, SFN, LFN slot, SFN" run of
// 32-byte directory entries in the image at path — the shape
// TestRootFileExistsSurvivesKernelDeletedEntries sets up — and overwrites the
// first byte of the earlier group's 3 slots with 0xE5, the FAT delete
// marker, leaving the later group untouched.
func deleteLFNGroup(t *testing.T, path string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading image to find the directory entries: %v", err)
	}

	const lfnAttr = 0x0f
	isLFN := func(offset int) bool { return raw[offset+11] == lfnAttr }

	found := -1
	for i := 0; i+5*32 <= len(raw); i += 32 {
		if isLFN(i) && isLFN(i+32) && !isLFN(i+64) && isLFN(i+96) && !isLFN(i+128) {
			found = i
			break
		}
	}
	if found == -1 {
		t.Fatal("could not find the expected LFN/SFN, LFN/SFN directory layout")
	}

	for slot := 0; slot < 3; slot++ {
		scribble(t, path, int64(found+slot*32), []byte{0xe5})
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

// fatFixture creates a whole-device FAT filesystem of the given go-diskfs
// width directly. diskfmt has no FormatFAT16/FormatFAT12 — GoSD's Format
// only ever writes FAT32 — so these fixtures stand in for a stick someone
// else formatted, the case gosd-8rw2 is about: Inspect must still name it
// honestly even though GoSD never created it.
func fatFixture(t *testing.T, sizeBytes int64, fsType filesystem.Type, label string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "device.img")
	d, err := diskfs.Create(path, sizeBytes, diskfs.SectorSize512)
	if err != nil {
		t.Fatalf("creating backing file: %v", err)
	}
	if _, err := d.CreateFilesystem(disk.FilesystemSpec{
		Partition:   0,
		FSType:      fsType,
		VolumeLabel: label,
	}); err != nil {
		t.Fatalf("creating fixture filesystem: %v", err)
	}
	if err := d.Close(); err != nil {
		t.Fatalf("closing fixture device: %v", err)
	}
	return path
}

// TestInspectReportsFAT16Label pins gosd-8rw2: before the fix, Inspect
// reported every FAT width as FAT32, so a refusal to overwrite a FAT16 stick
// named a filesystem that was not actually there.
func TestInspectReportsFAT16Label(t *testing.T) {
	path := fatFixture(t, 20*1024*1024, filesystem.TypeFat16, "OLDSTICK")

	got, err := Inspect(path)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if got.FS != FAT16 || got.Label != "OLDSTICK" {
		t.Errorf("Inspect of a FAT16 device = %+v, want {FS:fat16 Label:OLDSTICK}", got)
	}
	if got.FS.String() != "FAT16" {
		t.Errorf("FAT16.String() = %q, want %q", got.FS.String(), "FAT16")
	}
	if got.FS.MountType() != "vfat" {
		t.Errorf("FAT16.MountType() = %q, want %q", got.FS.MountType(), "vfat")
	}
}

// TestInspectReportsFAT12Label is TestInspectReportsFAT16Label's twin for the
// even narrower FAT width (typically tiny/floppy-sized volumes).
func TestInspectReportsFAT12Label(t *testing.T) {
	path := fatFixture(t, 1024*1024, filesystem.TypeFat12, "TINYFAT")

	got, err := Inspect(path)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if got.FS != FAT12 || got.Label != "TINYFAT" {
		t.Errorf("Inspect of a FAT12 device = %+v, want {FS:fat12 Label:TINYFAT}", got)
	}
	if got.FS.String() != "FAT12" {
		t.Errorf("FAT12.String() = %q, want %q", got.FS.String(), "FAT12")
	}
	if got.FS.MountType() != "vfat" {
		t.Errorf("FAT12.MountType() = %q, want %q", got.FS.MountType(), "vfat")
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
