package image_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
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

	report, err := image.Write(imgPath, image.Spec{
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
	if report.BootPartitionSizeBytes != image.DefaultBootPartitionSizeBytes {
		t.Errorf("report.BootPartitionSizeBytes = %d, want the default %d (BootSizeBytes unset)", report.BootPartitionSizeBytes, image.DefaultBootPartitionSizeBytes)
	}
	if want := int64(len(topLevel) + len(nested)); report.BootPartitionPayloadBytes != want {
		t.Errorf("report.BootPartitionPayloadBytes = %d, want %d (the boot files' total content length)", report.BootPartitionPayloadBytes, want)
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

	_, err := image.Write(imgPath, image.Spec{
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

// TestWriteFormatsBothPartitionsWithAddressableFATs guards the image against
// the FAT-sizing defect go-diskfs lays a FAT32 volume out with at ~0.8% of
// sizes: a volume that advertises more clusters than its FAT can index, which
// macOS First Aid and Windows chkdsk both call damaged. 64 MiB is one of the
// defective sizes, so a `--data-size` an app author might plausibly pick used
// to produce an image that fails a host's disk check on first mount.
func TestWriteFormatsBothPartitionsWithAddressableFATs(t *testing.T) {
	imgPath := filepath.Join(t.TempDir(), "test.img")

	const dataSizeBytes = 64 * 1024 * 1024
	if _, err := image.Write(imgPath, image.Spec{DataSizeBytes: dataSizeBytes}); err != nil {
		t.Fatalf("Write() failed: %v", err)
	}

	for _, part := range []struct {
		name   string
		offset int64
	}{
		{"GOSD-BOOT", bootPartitionOffsetBytes},
		{"GOSD-DATA", dataPartitionOffsetBytes},
	} {
		clusters, entries := fat32ClusterAndEntryCounts(t, imgPath, part.offset)
		if entries < clusters+2 {
			t.Errorf("%s advertises %d clusters but its FAT holds only %d entries; %d are needed to address them all",
				part.name, clusters, entries, clusters+2)
		}
	}
}

// fat32ClusterAndEntryCounts reads the FAT32 BIOS Parameter Block at offset and
// reports how many data clusters the volume advertises and how many of them one
// FAT can index — the arithmetic a host's disk check applies.
func fat32ClusterAndEntryCounts(t *testing.T, imgPath string, offset int64) (clusters, entries int64) {
	t.Helper()
	f, err := os.Open(imgPath)
	if err != nil {
		t.Fatalf("reopening the written image failed: %v", err)
	}
	defer func() { _ = f.Close() }()

	sector := make([]byte, 512)
	if _, err := f.ReadAt(sector, offset); err != nil {
		t.Fatalf("reading the boot sector at %d failed: %v", offset, err)
	}
	var (
		bytesPerSector    = int64(binary.LittleEndian.Uint16(sector[11:13]))
		sectorsPerCluster = int64(sector[13])
		reservedSectors   = int64(binary.LittleEndian.Uint16(sector[14:16]))
		fatCount          = int64(sector[16])
		totalSectors      = int64(binary.LittleEndian.Uint32(sector[32:36]))
		sectorsPerFAT     = int64(binary.LittleEndian.Uint32(sector[36:40]))
	)
	if sectorsPerCluster == 0 || sectorsPerFAT == 0 {
		t.Fatalf("the boot sector at %d is not a FAT32 one: % x", offset, sector[:64])
	}
	clusters = (totalSectors - reservedSectors - fatCount*sectorsPerFAT) / sectorsPerCluster
	return clusters, sectorsPerFAT * bytesPerSector / 4
}

func TestWriteRejectsRawWriteOverlappingDataPartition(t *testing.T) {
	imgPath := filepath.Join(t.TempDir(), "test.img")

	_, err := image.Write(imgPath, image.Spec{
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

	_, err := image.Write(imgPath, image.Spec{
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

	_, err := image.Write(imgPath, image.Spec{
		RawWrites: []image.RawWrite{
			{OffsetBytes: bootPartitionOffsetBytes, Content: bytes.NewReader([]byte("clobber"))},
		},
	})
	if !errors.Is(err, image.ErrRawWriteOverlap) {
		t.Fatalf("Write() with a raw write over partition 1 = %v, want an ErrRawWriteOverlap", err)
	}
}

func TestWriteRejectsTwoRawWritesThatOverlapEachOther(t *testing.T) {
	imgPath := filepath.Join(t.TempDir(), "test.img")

	// Mimics a Rockchip board's idbloader.img (offset 32768) growing past
	// its usual size and running into u-boot.itb's offset (8388608):
	// checkRawWriteBounds passes both individually (each lands cleanly in
	// the unpartitioned gap), but they clobber each other.
	const idbloaderOffset = 32768
	const ubootOffset = 8388608
	idbloader := bytes.Repeat([]byte{0xaa}, ubootOffset-idbloaderOffset+1)
	uboot := []byte("u-boot payload")

	_, err := image.Write(imgPath, image.Spec{
		RawWrites: []image.RawWrite{
			{OffsetBytes: idbloaderOffset, Content: bytes.NewReader(idbloader)},
			{OffsetBytes: ubootOffset, Content: bytes.NewReader(uboot)},
		},
	})
	if !errors.Is(err, image.ErrRawWriteOverlap) {
		t.Fatalf("Write() with two overlapping raw writes = %v, want an ErrRawWriteOverlap", err)
	}
	for _, want := range []string{"32768", "8388609", "8388608"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Write() error = %q, want it to name the offending offsets/lengths (missing %q)", err, want)
		}
	}
}

func TestWriteRejectsRawWriteStraddlingIntoBootPartition(t *testing.T) {
	imgPath := filepath.Join(t.TempDir(), "test.img")

	// Starts inside the gap but is long enough to run into partition 1.
	content := bytes.Repeat([]byte{0xff}, 1024)
	_, err := image.Write(imgPath, image.Spec{
		RawWrites: []image.RawWrite{
			{OffsetBytes: bootPartitionOffsetBytes - 512, Content: bytes.NewReader(content)},
		},
	})
	if !errors.Is(err, image.ErrRawWriteOverlap) {
		t.Fatalf("Write() with a raw write straddling into partition 1 = %v, want an ErrRawWriteOverlap", err)
	}
}

// TestWriteWithBootSizeMovesTheDataPartitionOffset is the acceptance test for
// bean gosd-m70t: a non-default Spec.BootSizeBytes must resize partition 1
// and shift partition 2 (and the image's total size) to start immediately
// after it, exactly as the fixed 256MiB default already did before
// --boot-size existed. Partition 1 still starts at the locked 16MiB offset -
// only its end moves.
func TestWriteWithBootSizeMovesTheDataPartitionOffset(t *testing.T) {
	imgPath := filepath.Join(t.TempDir(), "test.img")

	const (
		bootSizeBytes = 32 * 1024 * 1024
		dataSizeBytes = 4 * 1024 * 1024
	)

	report, err := image.Write(imgPath, image.Spec{
		BootFiles:     map[string]io.Reader{"gosd.toml": bytes.NewReader([]byte("contents\n"))},
		BootSizeBytes: bootSizeBytes,
		DataSizeBytes: dataSizeBytes,
	})
	if err != nil {
		t.Fatalf("Write() failed: %v", err)
	}
	if report.BootPartitionSizeBytes != bootSizeBytes {
		t.Errorf("report.BootPartitionSizeBytes = %d, want %d", report.BootPartitionSizeBytes, int64(bootSizeBytes))
	}

	d, err := diskfs.Open(imgPath, diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		t.Fatalf("reopening the written image failed: %v", err)
	}
	defer func() { _ = d.Close() }()

	part1, err := d.GetPartition(1)
	if err != nil {
		t.Fatalf("GetPartition(1) failed: %v", err)
	}
	if got := part1.GetStart(); got != bootPartitionOffsetBytes {
		t.Errorf("partition 1 starts at byte %d, want %d (16MiB, unaffected by BootSizeBytes)", got, bootPartitionOffsetBytes)
	}
	if got := part1.GetSize(); got != bootSizeBytes {
		t.Errorf("partition 1 size = %d bytes, want %d (BootSizeBytes)", got, int64(bootSizeBytes))
	}

	wantDataOffset := int64(bootPartitionOffsetBytes + bootSizeBytes)
	part2, err := d.GetPartition(2)
	if err != nil {
		t.Fatalf("GetPartition(2) failed: %v", err)
	}
	if got := part2.GetStart(); got != wantDataOffset {
		t.Errorf("partition 2 starts at byte %d, want %d (immediately after the resized partition 1)", got, wantDataOffset)
	}
}

// TestWriteReportsExactAbsoluteFileRanges is the acceptance test for the
// image-injection contract's core mechanism (gosd-49it): Spec.ReportRanges
// must come back as absolute, ordered, exact-content-length byte ranges
// that a caller can overwrite with a plain os.WriteAt and have the change
// visible at the FAT level - no FAT tooling involved on the writing side at
// all, exactly what a provisioning tool does.
func TestWriteReportsExactAbsoluteFileRanges(t *testing.T) {
	imgPath := filepath.Join(t.TempDir(), "test.img")

	const placeholderName = "placeholder.yaml"
	original := bytes.Repeat([]byte("0123456789"), 500) // exactly 5000 bytes

	report, err := image.Write(imgPath, image.Spec{
		BootFiles: map[string]io.Reader{
			"gosd.toml":     strings.NewReader("hostname = \"x\"\n"),
			placeholderName: bytes.NewReader(original),
		},
		ReportRanges: []string{placeholderName, "gosd.toml"},
	})
	if err != nil {
		t.Fatalf("Write() failed: %v", err)
	}

	ranges, ok := report.FileRanges[placeholderName]
	if !ok {
		t.Fatalf("report.FileRanges has no entry for %q; got %v", placeholderName, report.FileRanges)
	}
	if len(ranges) == 0 {
		t.Fatal("report.FileRanges has an empty range list")
	}

	// Two files can never share clusters on a consistent FAT volume, so
	// the placeholder's reported ranges must be disjoint from gosd.toml's
	// - the injection splice must not be able to clobber a neighbor.
	for _, pr := range ranges {
		for _, gr := range report.FileRanges["gosd.toml"] {
			if pr.OffsetBytes < gr.OffsetBytes+gr.LengthBytes && gr.OffsetBytes < pr.OffsetBytes+pr.LengthBytes {
				t.Errorf("placeholder range [%d, %d) overlaps gosd.toml's range [%d, %d)",
					pr.OffsetBytes, pr.OffsetBytes+pr.LengthBytes, gr.OffsetBytes, gr.OffsetBytes+gr.LengthBytes)
			}
		}
	}

	var total int64
	prevEnd := int64(-1)
	for _, r := range ranges {
		if r.OffsetBytes < bootPartitionOffsetBytes {
			t.Errorf("range offset %d is before the boot partition (starts at %d)", r.OffsetBytes, bootPartitionOffsetBytes)
		}
		if end := r.OffsetBytes + r.LengthBytes; end > bootPartitionOffsetBytes+image.DefaultBootPartitionSizeBytes {
			t.Errorf("range [%d, %d) runs past the end of the boot partition (%d bytes)", r.OffsetBytes, end, image.DefaultBootPartitionSizeBytes)
		}
		if prevEnd >= 0 && r.OffsetBytes < prevEnd {
			t.Errorf("ranges are not ordered: a range starting at %d follows one ending at %d", r.OffsetBytes, prevEnd)
		}
		prevEnd = r.OffsetBytes + r.LengthBytes
		total += r.LengthBytes
	}
	if total != int64(len(original)) {
		t.Errorf("ranges total %d bytes, want exactly %d (the file's content length)", total, len(original))
	}

	// Patch the reported ranges directly in the raw .img with same-length
	// replacement bytes via plain os.WriteAt - exactly what a browser-side
	// provisioning tool does, with no FAT code at all.
	const replacementUnit = "PATCHED-CONTENT-"
	replacement := bytes.Repeat([]byte(replacementUnit), (len(original)/len(replacementUnit))+1)[:len(original)]
	f, err := os.OpenFile(imgPath, os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("opening %s for patching: %v", imgPath, err)
	}
	var consumed int64
	for _, r := range ranges {
		if _, err := f.WriteAt(replacement[consumed:consumed+r.LengthBytes], r.OffsetBytes); err != nil {
			t.Fatalf("WriteAt(offset=%d, len=%d): %v", r.OffsetBytes, r.LengthBytes, err)
		}
		consumed += r.LengthBytes
	}
	if err := f.Close(); err != nil {
		t.Fatalf("closing %s after patching: %v", imgPath, err)
	}

	d, err := diskfs.Open(imgPath, diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		t.Fatalf("reopening the patched image failed: %v", err)
	}
	defer func() { _ = d.Close() }()

	fs, err := d.GetFilesystem(1)
	if err != nil {
		t.Fatalf("GetFilesystem(1) failed: %v", err)
	}
	got, err := fs.ReadFile(placeholderName)
	if err != nil {
		t.Fatalf("reading %s back at the FAT level failed: %v", placeholderName, err)
	}
	if !bytes.Equal(got, replacement) {
		t.Errorf("FAT-level content after patching = %q, want the replacement %q", got, replacement)
	}
}

// TestWriteRejectsReportRangesPathNotInBootFiles confirms a typo'd
// ReportRanges entry fails cheaply, before any image bytes exist, rather
// than surfacing as an obscure go-diskfs error mid-write.
func TestWriteRejectsReportRangesPathNotInBootFiles(t *testing.T) {
	imgPath := filepath.Join(t.TempDir(), "test.img")

	_, err := image.Write(imgPath, image.Spec{
		BootFiles:    map[string]io.Reader{"gosd.toml": strings.NewReader("hostname = \"x\"\n")},
		ReportRanges: []string{"not-a-boot-file.yaml"},
	})
	if err == nil {
		t.Fatal("Write() with a ReportRanges path absent from BootFiles succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "not-a-boot-file.yaml") {
		t.Errorf("error = %q, want it to mention the offending path", err)
	}
	if _, statErr := os.Stat(imgPath); !os.IsNotExist(statErr) {
		t.Errorf("Write() wrote %s despite refusing ReportRanges; the refusal must come before any image bytes are written", imgPath)
	}
}

// TestWriteWrapsGoDiskfsDiskFullError is the acceptance test for gosd-m70t's
// fit reporting: a boot partition too small for its BootFiles must fail with
// image.ErrBootPartitionFull, not go-diskfs's bare "no space left on device"
// - which names no flag and gives a developer no way to know what to change.
func TestWriteWrapsGoDiskfsDiskFullError(t *testing.T) {
	imgPath := filepath.Join(t.TempDir(), "test.img")

	// The smallest FAT32 volume go-diskfs will format at all; a single
	// modest file still can't fit once the reserved area and root
	// directory are accounted for.
	const tinyBootSizeBytes = 1024 * 1024
	payload := bytes.Repeat([]byte{0xaa}, 2*1024*1024)

	_, err := image.Write(imgPath, image.Spec{
		BootFiles:     map[string]io.Reader{"big-file.bin": bytes.NewReader(payload)},
		BootSizeBytes: tinyBootSizeBytes,
	})
	if !errors.Is(err, image.ErrBootPartitionFull) {
		t.Fatalf("Write() with an oversized payload = %v, want an ErrBootPartitionFull", err)
	}
}
