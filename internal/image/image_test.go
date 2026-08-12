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

	"github.com/jphastings/gosd/internal/diskfmt"
	"github.com/jphastings/gosd/internal/image"
)

const (
	bootPartitionOffsetBytes = 16 * 1024 * 1024  // locked layout: partition 1 starts at 16MiB
	dataPartitionOffsetBytes = 272 * 1024 * 1024 // locked layout: partition 2 starts right after partition 1

	// The labels a `gosd build` for an app called "test" would resolve
	// (see internal/naming.LabelsFor): required Spec fields, with nothing
	// special about these particular values.
	testBootLabel = "test-boot"
	testDataLabel = "test-data"
)

func TestWriteProducesAReadableImage(t *testing.T) {
	imgPath := filepath.Join(t.TempDir(), "test.img")

	topLevel := []byte("gosd.toml contents\n")
	nested := []byte("nested boot script contents\n")
	raw := []byte("raw-bootloader-payload")

	report, err := image.Write(imgPath, image.Spec{
		BootLabel: testBootLabel,
		DataLabel: testDataLabel,
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
	if label := strings.TrimSpace(fs.Label()); label != testBootLabel {
		t.Errorf("boot partition label = %q, want %q (Spec.BootLabel, verbatim)", label, testBootLabel)
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
		BootLabel:     testBootLabel,
		DataLabel:     testDataLabel,
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
	if label := strings.TrimSpace(fs.Label()); label != testDataLabel {
		t.Errorf("data partition label = %q, want %q (Spec.DataLabel, verbatim)", label, testDataLabel)
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
	if _, err := image.Write(imgPath, image.Spec{BootLabel: testBootLabel, DataLabel: testDataLabel, DataSizeBytes: dataSizeBytes}); err != nil {
		t.Fatalf("Write() failed: %v", err)
	}

	for _, part := range []struct {
		name   string
		offset int64
	}{
		{testBootLabel, bootPartitionOffsetBytes},
		{testDataLabel, dataPartitionOffsetBytes},
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

// TestWriteRefusesUnusableLabelsBeforeCreatingTheImage covers the guard
// go-diskfs cannot: it formats a volume label through a "%-11.11s" verb, so
// an over-long label would silently ship as a truncated one - an image
// labelled something other than what its own config.json tells gosd-init to
// look for, which reformats the data partition on the next boot. An empty
// label is the same class of caller bug (see internal/naming.LabelsFor and
// `gosd build --label-prefix`). Both must be refused before any image file
// exists at all, so a failed build leaves nothing behind.
func TestWriteRefusesUnusableLabelsBeforeCreatingTheImage(t *testing.T) {
	cases := []struct {
		name string
		spec image.Spec
	}{
		{"an empty boot label", image.Spec{DataLabel: testDataLabel}},
		{"an over-long boot label", image.Spec{BootLabel: "twelvechars!", DataLabel: testDataLabel}},
		{
			name: "an empty data label with a data partition",
			spec: image.Spec{BootLabel: testBootLabel, DataSizeBytes: 4 * 1024 * 1024},
		},
		{
			name: "an over-long data label with a data partition",
			spec: image.Spec{BootLabel: testBootLabel, DataLabel: "twelvechars!", DataSizeBytes: 4 * 1024 * 1024},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			imgPath := filepath.Join(t.TempDir(), "test.img")
			if _, err := image.Write(imgPath, c.spec); err == nil {
				t.Fatal("Write() = nil, want a refusal naming the offending Spec field")
			}
			if _, err := os.Stat(imgPath); !os.IsNotExist(err) {
				t.Errorf("os.Stat(%s) = %v, want the image never to have been created", imgPath, err)
			}
		})
	}
}

// An empty Spec.DataLabel is only a problem when there is a partition 2 to
// label - a --data-size=0 build has none, and needn't invent one.
func TestWriteAcceptsAnEmptyDataLabelWithNoDataPartition(t *testing.T) {
	imgPath := filepath.Join(t.TempDir(), "test.img")

	if _, err := image.Write(imgPath, image.Spec{BootLabel: testBootLabel}); err != nil {
		t.Fatalf("Write() = %v, want nil", err)
	}
}

// Labels are lowercase per-app names now, and nothing in the stack may
// upper-case, trim or otherwise rewrite them - a label that doesn't round
// trip is a data partition reformatted on the next boot. 11 bytes is FAT's
// maximum, the case most likely to be mangled.
func TestWriteRoundTripsLabelsVerbatim(t *testing.T) {
	pairs := map[string]struct{ boot, data string }{
		// 11 bytes each: the longest prefix (6) plus a 5-byte suffix.
		"the longest labels FAT allows": {"eleven-boot", "eleven-data"},
		// `gosd build --label-prefix` is used exactly as typed, so a
		// mixed-case prefix must survive the FAT formatter unaltered too.
		"a mixed-case prefix": {"Web-boot", "Web-data"},
	}
	for name, pair := range pairs {
		t.Run(name, func(t *testing.T) {
			imgPath := filepath.Join(t.TempDir(), "test.img")
			if _, err := image.Write(imgPath, image.Spec{
				BootLabel:     pair.boot,
				DataLabel:     pair.data,
				DataSizeBytes: 64 * 1024 * 1024,
			}); err != nil {
				t.Fatalf("Write() failed: %v", err)
			}

			d, err := diskfs.Open(imgPath, diskfs.WithOpenMode(diskfs.ReadOnly))
			if err != nil {
				t.Fatalf("reopening the written image failed: %v", err)
			}
			defer func() { _ = d.Close() }()

			for _, part := range []struct {
				index int
				want  string
			}{{1, pair.boot}, {2, pair.data}} {
				fs, err := d.GetFilesystem(part.index)
				if err != nil {
					t.Fatalf("GetFilesystem(%d) failed: %v", part.index, err)
				}
				if got := strings.TrimSpace(fs.Label()); got != part.want {
					t.Errorf("partition %d label = %q, want %q unchanged", part.index, got, part.want)
				}
			}
		})
	}
}

func TestWriteRejectsRawWriteOverlappingDataPartition(t *testing.T) {
	imgPath := filepath.Join(t.TempDir(), "test.img")

	_, err := image.Write(imgPath, image.Spec{
		BootLabel:     testBootLabel,
		DataLabel:     testDataLabel,
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
		BootLabel: testBootLabel,
		DataLabel: testDataLabel,
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
		BootLabel: testBootLabel,
		DataLabel: testDataLabel,
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
		BootLabel: testBootLabel,
		DataLabel: testDataLabel,
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
		BootLabel: testBootLabel,
		DataLabel: testDataLabel,
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
		BootLabel:     testBootLabel,
		DataLabel:     testDataLabel,
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
		BootLabel: testBootLabel,
		DataLabel: testDataLabel,
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
		BootLabel:    testBootLabel,
		DataLabel:    testDataLabel,
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
		BootLabel:     testBootLabel,
		DataLabel:     testDataLabel,
		BootFiles:     map[string]io.Reader{"big-file.bin": bytes.NewReader(payload)},
		BootSizeBytes: tinyBootSizeBytes,
	})
	if !errors.Is(err, image.ErrBootPartitionFull) {
		t.Fatalf("Write() with an oversized payload = %v, want an ErrBootPartitionFull", err)
	}
}

// mbrPartitionType returns the MBR type byte the partition table records for
// partition index, failing the test if the table has no entry for it.
func mbrPartitionType(t *testing.T, imgPath string, index int) mbr.Type {
	t.Helper()
	d, err := diskfs.Open(imgPath, diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		t.Fatalf("reopening the written image failed: %v", err)
	}
	defer func() { _ = d.Close() }()

	table, err := d.GetPartitionTable()
	if err != nil {
		t.Fatalf("GetPartitionTable() failed: %v", err)
	}
	mbrTable, ok := table.(*mbr.Table)
	if !ok {
		t.Fatalf("GetPartitionTable() returned %T, want *mbr.Table", table)
	}
	for _, p := range mbrTable.Partitions {
		if p.Index == index {
			return p.Type
		}
	}
	t.Fatalf("mbr table has no entry for partition %d", index)
	return 0
}

// extractRegion copies length bytes starting at offset out of imgPath into a
// new temp file and returns its path: diskfmt.Inspect takes a device path,
// not a byte range within a larger file, so reading back the data
// partition's own filesystem needs it isolated first.
func extractRegion(t *testing.T, imgPath string, offset, length int64) string {
	t.Helper()
	src, err := os.Open(imgPath)
	if err != nil {
		t.Fatalf("opening %s to extract a region: %v", imgPath, err)
	}
	defer func() { _ = src.Close() }()

	regionPath := filepath.Join(t.TempDir(), "region.img")
	dst, err := os.Create(regionPath)
	if err != nil {
		t.Fatalf("creating %s: %v", regionPath, err)
	}
	if _, err := io.Copy(dst, io.NewSectionReader(src, offset, length)); err != nil {
		t.Fatalf("copying region [%d, %d) from %s: %v", offset, offset+length, imgPath, err)
	}
	if err := dst.Close(); err != nil {
		t.Fatalf("closing %s: %v", regionPath, err)
	}
	return regionPath
}

// TestWriteWithEXT4DataFilesystemProducesAReadableEXT4Partition is the
// acceptance test for bean gosd-95yu's image half: DataFilesystem: EXT4 must
// mark partition 2 as Linux (0x83), not FAT32 (0x0C), and its bytes must be a
// real ext4 filesystem labelled with Spec.DataLabel that diskfmt.Inspect can read back.
func TestWriteWithEXT4DataFilesystemProducesAReadableEXT4Partition(t *testing.T) {
	imgPath := filepath.Join(t.TempDir(), "test.img")
	dataSizeBytes := diskfmt.MinEXT4Bytes()

	_, err := image.Write(imgPath, image.Spec{
		BootLabel:      testBootLabel,
		DataLabel:      testDataLabel,
		DataSizeBytes:  dataSizeBytes,
		DataFilesystem: diskfmt.EXT4,
	})
	if err != nil {
		t.Fatalf("Write() failed: %v", err)
	}

	d, err := diskfs.Open(imgPath, diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		t.Fatalf("reopening the written image failed: %v", err)
	}
	part, err := d.GetPartition(2)
	if closeErr := d.Close(); closeErr != nil {
		t.Fatalf("closing the reopened image failed: %v", closeErr)
	}
	if err != nil {
		t.Fatalf("GetPartition(2) failed: %v", err)
	}
	if got := part.GetStart(); got != dataPartitionOffsetBytes {
		t.Errorf("partition 2 starts at byte %d, want %d (immediately after partition 1)", got, int64(dataPartitionOffsetBytes))
	}
	if got := part.GetSize(); got != dataSizeBytes {
		t.Errorf("partition 2 size = %d bytes, want %d", got, dataSizeBytes)
	}

	if gotType := mbrPartitionType(t, imgPath, 2); gotType != mbr.Linux {
		t.Errorf("partition 2 type = %#x, want %#x (Linux/ext4)", byte(gotType), byte(mbr.Linux))
	}

	regionPath := extractRegion(t, imgPath, dataPartitionOffsetBytes, dataSizeBytes)
	contents, err := diskfmt.Inspect(regionPath)
	if err != nil {
		t.Fatalf("diskfmt.Inspect on the extracted data partition failed: %v", err)
	}
	if contents.FS != diskfmt.EXT4 {
		t.Errorf("Inspect().FS = %v, want ext4", contents.FS)
	}
	if contents.Label != testDataLabel {
		t.Errorf("Inspect().Label = %q, want %q (Spec.DataLabel, stamped into the ext4 golden too)", contents.Label, testDataLabel)
	}
}

// TestWriteDefaultDataFilesystemIsStillFAT32 pins the zero value of
// Spec.DataFilesystem to FAT32 explicitly, so introducing DataFilesystem
// (bean gosd-95yu) cannot silently flip existing callers' images to ext4.
func TestWriteDefaultDataFilesystemIsStillFAT32(t *testing.T) {
	imgPath := filepath.Join(t.TempDir(), "test.img")
	const dataSizeBytes = 4 * 1024 * 1024

	if _, err := image.Write(imgPath, image.Spec{BootLabel: testBootLabel, DataLabel: testDataLabel, DataSizeBytes: dataSizeBytes}); err != nil {
		t.Fatalf("Write() failed: %v", err)
	}

	if gotType := mbrPartitionType(t, imgPath, 2); gotType != mbr.Fat32LBA {
		t.Errorf("partition 2 type = %#x with Spec.DataFilesystem unset, want %#x (FAT32)", byte(gotType), byte(mbr.Fat32LBA))
	}
}

// TestWriteRejectsEXT4DataSizeBelowTheGoldenMinimum confirms an ext4 data
// partition too small for the golden image is refused before any image
// bytes are written, actionably (naming ext4 and the shortfall) rather than
// failing deep inside diskfmt.WriteEXT4.
func TestWriteRejectsEXT4DataSizeBelowTheGoldenMinimum(t *testing.T) {
	imgPath := filepath.Join(t.TempDir(), "test.img")

	_, err := image.Write(imgPath, image.Spec{
		BootLabel:      testBootLabel,
		DataLabel:      testDataLabel,
		DataSizeBytes:  diskfmt.MinEXT4Bytes() - 1,
		DataFilesystem: diskfmt.EXT4,
	})
	if err == nil {
		t.Fatal("Write() with an ext4 data size below the golden minimum succeeded, want a refusal")
	}
	if !strings.Contains(err.Error(), "ext4") {
		t.Errorf("error = %q, want it to mention ext4", err)
	}
	if _, statErr := os.Stat(imgPath); !os.IsNotExist(statErr) {
		t.Errorf("Write() wrote %s despite refusing the undersized ext4 data partition", imgPath)
	}
}

// TestWriteRejectsAnUnsupportedDataFilesystem confirms a Spec.DataFilesystem
// value other than the zero value, FAT32 or EXT4 (e.g. exFAT, not yet wired
// up here) is refused by name, before any image bytes are written.
func TestWriteRejectsAnUnsupportedDataFilesystem(t *testing.T) {
	imgPath := filepath.Join(t.TempDir(), "test.img")

	_, err := image.Write(imgPath, image.Spec{
		BootLabel:      testBootLabel,
		DataLabel:      testDataLabel,
		DataSizeBytes:  4 * 1024 * 1024,
		DataFilesystem: diskfmt.ExFAT,
	})
	if err == nil {
		t.Fatal("Write() with DataFilesystem: diskfmt.ExFAT succeeded, want a refusal")
	}
	if !strings.Contains(err.Error(), "DataFilesystem") {
		t.Errorf("error = %q, want it to name Spec.DataFilesystem", err)
	}
	if _, statErr := os.Stat(imgPath); !os.IsNotExist(statErr) {
		t.Errorf("Write() wrote %s despite refusing the unsupported filesystem", imgPath)
	}
}

// TestWriteWithEXT4DoesNotApplyTheFAT32SizingTrim guards the ext4 path
// against inheriting FAT32's go-diskfs sizing workaround
// (LargestSelfConsistentFAT32Bytes): an ext4 data partition has no such
// go-diskfs-shaped defect, so a size that would be silently trimmed under
// FAT32 must come through untrimmed for ext4.
func TestWriteWithEXT4DoesNotApplyTheFAT32SizingTrim(t *testing.T) {
	imgPath := filepath.Join(t.TempDir(), "test.img")
	dataSizeBytes := firstFAT32TrimmedSizeAtOrAbove(t, diskfmt.MinEXT4Bytes())

	_, err := image.Write(imgPath, image.Spec{
		BootLabel:      testBootLabel,
		DataLabel:      testDataLabel,
		DataSizeBytes:  dataSizeBytes,
		DataFilesystem: diskfmt.EXT4,
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
	if got := part.GetSize(); got != dataSizeBytes {
		t.Errorf("partition 2 size = %d bytes, want the untrimmed %d - the FAT32 sizing trim must not apply to an ext4 data partition", got, dataSizeBytes)
	}
}

// firstFAT32TrimmedSizeAtOrAbove searches upward in whole sectors from
// minBytes for the first size diskfmt.LargestSelfConsistentFAT32Bytes would
// actually trim - proof material for
// TestWriteWithEXT4DoesNotApplyTheFAT32SizingTrim, found by direct
// construction against the real function rather than a hardcoded byte
// count, since exactly which sizes are defective moves with sectors-per-FAT
// rounding (that function's own doc explains why). Every FAT32 sizing band
// go-diskfs lays out is defective at its very top, so a window a few bands
// wide is always enough to find one.
func firstFAT32TrimmedSizeAtOrAbove(t *testing.T, minBytes int64) int64 {
	t.Helper()
	const sectorSizeBytes = 512
	const searchWindowBytes = 8 * 1024 * 1024
	for candidate := minBytes; candidate < minBytes+searchWindowBytes; candidate += sectorSizeBytes {
		if diskfmt.LargestSelfConsistentFAT32Bytes(candidate) != candidate {
			return candidate
		}
	}
	t.Fatalf("no FAT32-defective size found within %d bytes above %d; the search window may need widening", searchWindowBytes, minBytes)
	return 0
}
