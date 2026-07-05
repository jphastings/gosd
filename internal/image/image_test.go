package image_test

import (
	"bytes"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	diskfs "github.com/diskfs/go-diskfs"
	"github.com/diskfs/go-diskfs/partition/mbr"

	"github.com/jphastings/gosd/internal/image"
)

const (
	bootPartitionOffsetBytes = 16 * 1024 * 1024  // locked layout: partition 1 starts at 16MiB
	dataPartitionOffsetBytes = 272 * 1024 * 1024 // locked layout: partition 2 starts right after partition 1
)

func TestWriteProducesAReadableImage(t *testing.T) {
	imgPath := filepath.Join(t.TempDir(), "test.img")

	topLevel := []byte("gosd.toml contents\n")
	nested := []byte("nested boot script contents\n")
	raw := []byte("raw-bootloader-payload")

	err := image.Write(imgPath, image.Spec{
		BootFiles: map[string]io.Reader{
			"gosd.toml":           bytes.NewReader(topLevel),
			"nested/dir/boot.scr": bytes.NewReader(nested),
		},
		RawWrites: []image.RawWrite{
			// LBA 64 at 512-byte sectors, per the bean's acceptance test.
			{OffsetBytes: 64 * 512, Content: bytes.NewReader(raw)},
		},
	})
	if err != nil {
		t.Fatalf("Write() failed: %v", err)
	}

	d, err := diskfs.Open(imgPath, diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		t.Fatalf("reopening the written image failed: %v", err)
	}
	defer func() { _ = d.Close() }()

	part, err := d.GetPartition(1)
	if err != nil {
		t.Fatalf("GetPartition(1) failed: %v", err)
	}
	if got := part.GetStart(); got != bootPartitionOffsetBytes {
		t.Errorf("partition 1 starts at byte %d, want %d (16MiB)", got, bootPartitionOffsetBytes)
	}
	fs, err := d.GetFilesystem(1)
	if err != nil {
		t.Fatalf("GetFilesystem(1) failed: %v", err)
	}
	if label := strings.TrimSpace(fs.Label()); label != "GOSD-BOOT" {
		t.Errorf("boot partition label = %q, want GOSD-BOOT", label)
	}

	gotTop, err := fs.ReadFile("gosd.toml")
	if err != nil {
		t.Fatalf("reading gosd.toml back failed: %v", err)
	}
	if !bytes.Equal(gotTop, topLevel) {
		t.Errorf("gosd.toml contents = %q, want %q", gotTop, topLevel)
	}

	gotNested, err := fs.ReadFile("nested/dir/boot.scr")
	if err != nil {
		t.Fatalf("reading nested/dir/boot.scr back failed: %v", err)
	}
	if !bytes.Equal(gotNested, nested) {
		t.Errorf("nested/dir/boot.scr contents = %q, want %q", gotNested, nested)
	}

	gotRaw := make([]byte, len(raw))
	if _, err := d.Backend.ReadAt(gotRaw, 64*512); err != nil {
		t.Fatalf("reading back the raw write failed: %v", err)
	}
	if !bytes.Equal(gotRaw, raw) {
		t.Errorf("raw write contents = %q, want %q", gotRaw, raw)
	}

	// The MBR always has 4 on-disk partition entry slots; unused ones read
	// back as a zero-sized entry rather than an error, so the way to assert
	// "no partition 2" is a zero size, not GetPartition failing.
	if part2, err := d.GetPartition(2); err == nil && part2.GetSize() != 0 {
		t.Errorf("partition 2 has size %d with DataSizeBytes unset, want the single-partition layout (no partition 2)", part2.GetSize())
	}
}

func TestWriteWithDataSizeAddsASecondFat32Partition(t *testing.T) {
	imgPath := filepath.Join(t.TempDir(), "test.img")

	const dataSizeBytes = 4 * 1024 * 1024 // small, so the test doesn't need a full 1GiB partition

	err := image.Write(imgPath, image.Spec{
		BootFiles:     map[string]io.Reader{"gosd.toml": bytes.NewReader([]byte("contents\n"))},
		DataSizeBytes: dataSizeBytes,
	})
	if err != nil {
		t.Fatalf("Write() failed: %v", err)
	}

	d, err := diskfs.Open(imgPath, diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		t.Fatalf("reopening the written image failed: %v", err)
	}
	defer func() { _ = d.Close() }()

	part, err := d.GetPartition(2)
	if err != nil {
		t.Fatalf("GetPartition(2) failed: %v", err)
	}
	if got := part.GetStart(); got != dataPartitionOffsetBytes {
		t.Errorf("partition 2 starts at byte %d, want %d (immediately after partition 1)", got, dataPartitionOffsetBytes)
	}
	if got := part.GetSize(); got != dataSizeBytes {
		t.Errorf("partition 2 size = %d bytes, want %d", got, int64(dataSizeBytes))
	}

	table, err := d.GetPartitionTable()
	if err != nil {
		t.Fatalf("GetPartitionTable() failed: %v", err)
	}
	mbrTable, ok := table.(*mbr.Table)
	if !ok {
		t.Fatalf("GetPartitionTable() returned %T, want *mbr.Table", table)
	}
	var gotType mbr.Type
	found := false
	for _, p := range mbrTable.Partitions {
		if p.Index == 2 {
			gotType = p.Type
			found = true
		}
	}
	if !found {
		t.Fatal("mbr table has no entry for partition 2")
	}
	if gotType != mbr.Fat32LBA {
		t.Errorf("partition 2 type = %#x, want %#x (FAT32 LBA)", byte(gotType), byte(mbr.Fat32LBA))
	}

	fs, err := d.GetFilesystem(2)
	if err != nil {
		t.Fatalf("GetFilesystem(2) failed: %v", err)
	}
	if label := strings.TrimSpace(fs.Label()); label != "GOSD-DATA" {
		t.Errorf("data partition label = %q, want GOSD-DATA", label)
	}

	// Partition 1 must still be intact and untouched by the new partition.
	fs1, err := d.GetFilesystem(1)
	if err != nil {
		t.Fatalf("GetFilesystem(1) failed: %v", err)
	}
	if got, err := fs1.ReadFile("gosd.toml"); err != nil || string(got) != "contents\n" {
		t.Errorf("boot partition contents = (%q, %v), want (\"contents\\n\", nil)", got, err)
	}
}

func TestWriteRejectsRawWriteOverlappingDataPartition(t *testing.T) {
	imgPath := filepath.Join(t.TempDir(), "test.img")

	err := image.Write(imgPath, image.Spec{
		DataSizeBytes: 4 * 1024 * 1024,
		RawWrites: []image.RawWrite{
			{OffsetBytes: dataPartitionOffsetBytes, Content: bytes.NewReader([]byte("clobber"))},
		},
	})
	if !errors.Is(err, image.ErrRawWriteOverlap) {
		t.Fatalf("Write() with a raw write over partition 2 = %v, want an ErrRawWriteOverlap", err)
	}
}

func TestWriteRejectsRawWriteOverlappingMBR(t *testing.T) {
	imgPath := filepath.Join(t.TempDir(), "test.img")

	err := image.Write(imgPath, image.Spec{
		RawWrites: []image.RawWrite{
			{OffsetBytes: 0, Content: bytes.NewReader([]byte("clobber"))},
		},
	})
	if !errors.Is(err, image.ErrRawWriteOverlap) {
		t.Fatalf("Write() with a raw write over the MBR = %v, want an ErrRawWriteOverlap", err)
	}
}

func TestWriteRejectsRawWriteOverlappingBootPartition(t *testing.T) {
	imgPath := filepath.Join(t.TempDir(), "test.img")

	err := image.Write(imgPath, image.Spec{
		RawWrites: []image.RawWrite{
			{OffsetBytes: bootPartitionOffsetBytes, Content: bytes.NewReader([]byte("clobber"))},
		},
	})
	if !errors.Is(err, image.ErrRawWriteOverlap) {
		t.Fatalf("Write() with a raw write over partition 1 = %v, want an ErrRawWriteOverlap", err)
	}
}

func TestWriteRejectsRawWriteStraddlingIntoBootPartition(t *testing.T) {
	imgPath := filepath.Join(t.TempDir(), "test.img")

	// Starts inside the gap but is long enough to run into partition 1.
	content := bytes.Repeat([]byte{0xff}, 1024)
	err := image.Write(imgPath, image.Spec{
		RawWrites: []image.RawWrite{
			{OffsetBytes: bootPartitionOffsetBytes - 512, Content: bytes.NewReader(content)},
		},
	})
	if !errors.Is(err, image.ErrRawWriteOverlap) {
		t.Fatalf("Write() with a raw write straddling into partition 1 = %v, want an ErrRawWriteOverlap", err)
	}
}
