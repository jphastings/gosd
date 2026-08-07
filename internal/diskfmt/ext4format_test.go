package diskfmt

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"testing"

	"github.com/jphastings/gosd/internal/diskfmt/ext4golden"
)

// TestFormatEXT4RoundTripsThroughInspect is the headline proof: a volume this
// package wrote is one this package reads back correctly, label and UUID
// both, mirroring TestFormatExFATRoundTripsThroughInspect.
func TestFormatEXT4RoundTripsThroughInspect(t *testing.T) {
	path := backingFile(t, ext4golden.RawBytes)
	if err := FormatEXT4(path, "GOSD-DATA"); err != nil {
		t.Fatalf("FormatEXT4: %v", err)
	}

	got, err := Inspect(path)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if got.FS != EXT4 || got.Label != "GOSD-DATA" {
		t.Errorf("Inspect = %+v, want {FS:ext4 Label:GOSD-DATA}", got)
	}
	if got.UUID == "" || got.UUID == ext4GoldenPlaceholderUUID {
		t.Errorf("Inspect UUID = %q, want a freshly generated one (not empty, not the golden's placeholder)", got.UUID)
	}
}

// ext4GoldenPlaceholderUUID is the fixed UUID the golden image ships with
// (internal/diskfmt/ext4golden/manifest.json) before FormatEXT4 stamps a
// real one — a stamped volume must never still carry it.
const ext4GoldenPlaceholderUUID = "4c1a41c8-20b8-4c50-8399-7fae324e8398"

func TestFormatEXT4RefusesAnOverlongLabel(t *testing.T) {
	path := backingFile(t, ext4golden.RawBytes)
	if err := FormatEXT4(path, "SEVENTEEN-CHARS!!"); err == nil {
		t.Fatal("FormatEXT4 accepted a 17-byte label")
	}
}

func TestFormatEXT4RefusesATargetSmallerThanTheGoldenImage(t *testing.T) {
	path := backingFile(t, ext4golden.RawBytes-1)
	if err := FormatEXT4(path, "GOSD-DATA"); err == nil {
		t.Fatal("FormatEXT4 accepted a device one byte smaller than the golden image")
	}
}

// TestFormatEXT4GeneratesAFreshUUIDEachTime guards against a static or
// zero-valued UUID slipping in: two formats of two different files must not
// collide.
func TestFormatEXT4GeneratesAFreshUUIDEachTime(t *testing.T) {
	path1 := backingFile(t, ext4golden.RawBytes)
	path2 := backingFile(t, ext4golden.RawBytes)
	if err := FormatEXT4(path1, "GOSD-DATA"); err != nil {
		t.Fatalf("FormatEXT4 (1): %v", err)
	}
	if err := FormatEXT4(path2, "GOSD-DATA"); err != nil {
		t.Fatalf("FormatEXT4 (2): %v", err)
	}

	c1, err := Inspect(path1)
	if err != nil {
		t.Fatalf("Inspect (1): %v", err)
	}
	c2, err := Inspect(path2)
	if err != nil {
		t.Fatalf("Inspect (2): %v", err)
	}
	if c1.UUID == c2.UUID {
		t.Errorf("two independent FormatEXT4 calls produced the same UUID: %s", c1.UUID)
	}
}

// TestFormatEXT4StampsBackupSuperblocksConsistently reads the raw backup
// superblock copies back off the device (bypassing Inspect, which only ever
// looks at the primary) and checks each carries the same stamped UUID/label
// as the primary and a checksum that verifies against its own bytes — the
// same thing tune2fs -U/-L does, confirmed empirically against real
// e2fsprogs while building this package (see the bean's Summary of
// Changes).
func TestFormatEXT4StampsBackupSuperblocksConsistently(t *testing.T) {
	path := backingFile(t, ext4golden.RawBytes)
	if err := FormatEXT4(path, "GOSD-DATA"); err != nil {
		t.Fatalf("FormatEXT4: %v", err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("reopening formatted device: %v", err)
	}
	defer func() { _ = f.Close() }()

	primary := readSuperblockAt(t, f, ext4SuperblockOffset)
	sb, err := parseEXT4Superblock(primary)
	if err != nil {
		t.Fatalf("parsing the primary superblock: %v", err)
	}

	backups, err := ext4BackupSuperblockOffsets(sb)
	if err != nil {
		t.Fatalf("ext4BackupSuperblockOffsets: %v", err)
	}
	if len(backups) == 0 {
		t.Fatal("golden image has no backup superblocks to check — test fixture assumption broken")
	}

	for _, off := range backups {
		backup := readSuperblockAt(t, f, off)
		if !bytes.Equal(backup[ext4SuperblockOffUUID:ext4SuperblockOffUUID+16], primary[ext4SuperblockOffUUID:ext4SuperblockOffUUID+16]) {
			t.Errorf("backup superblock at %d has a different UUID than the primary", off)
		}
		if !bytes.Equal(backup[ext4SuperblockOffVolumeName:ext4SuperblockOffVolumeName+ext4LabelBytes], primary[ext4SuperblockOffVolumeName:ext4SuperblockOffVolumeName+ext4LabelBytes]) {
			t.Errorf("backup superblock at %d has a different label than the primary", off)
		}
		stored := binary.LittleEndian.Uint32(backup[ext4SuperblockOffChecksum:])
		recomputed := ext4Checksum(0xFFFFFFFF, backup[:ext4SuperblockOffChecksum])
		if stored != recomputed {
			t.Errorf("backup superblock at %d: stored checksum 0x%08X does not match its own bytes (0x%08X)", off, stored, recomputed)
		}
	}
}

func readSuperblockAt(t *testing.T, f *os.File, off int64) []byte {
	t.Helper()
	buf := make([]byte, ext4SuperblockSize)
	if _, err := f.ReadAt(buf, off); err != nil {
		t.Fatalf("reading superblock at %d: %v", off, err)
	}
	return buf
}

// failingWriterAt fails once a write would reach failAt, simulating a device
// write error (e.g. a full disk) partway through a format — the crash- /
// error-ordering case the bean asks to prove is handled honestly: a
// truncated write to the target must surface as a real error, never a
// silent success.
type failingWriterAt struct {
	w      *os.File
	failAt int64
}

func (f *failingWriterAt) WriteAt(p []byte, off int64) (int, error) {
	if off+int64(len(p)) > f.failAt {
		return 0, errors.New("simulated write failure (disk full)")
	}
	return f.w.WriteAt(p, off)
}

func TestWriteEXT4FailsHonestlyWhenTheUnderlyingWriteFails(t *testing.T) {
	path := backingFile(t, ext4golden.RawBytes)
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("opening backing file: %v", err)
	}
	defer func() { _ = f.Close() }()

	fw := &failingWriterAt{w: f, failAt: 2 << 20} // fail partway through the first few chunks
	if err := writeEXT4(fw, ext4golden.RawBytes, "GOSD-DATA"); err == nil {
		t.Fatal("writeEXT4 with a failing underlying writer = nil error, want a failure")
	}
}
