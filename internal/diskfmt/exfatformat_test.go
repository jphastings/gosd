package diskfmt

import (
	"bytes"
	"encoding/binary"
	"os"
	"slices"
	"testing"
)

// volume is a formatted image opened for inspection. The images under test run
// to tens of gigabytes (sparse), so tests read the spans they care about rather
// than the whole file.
type volume struct {
	t    *testing.T
	f    *os.File
	path string
	g    exFATGeometry
}

func formatted(t *testing.T, sizeBytes int64, label string) *volume {
	t.Helper()
	path := backingFile(t, sizeBytes)
	if err := FormatExFAT(path, label); err != nil {
		t.Fatalf("FormatExFAT: %v", err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("reopening the formatted volume: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })

	v := &volume{t: t, f: f, path: path}
	g, err := parseExFATBootSector(v.at(0, exFATBootSectorBytes))
	if err != nil {
		t.Fatalf("the volume we just wrote does not parse: %v", err)
	}
	v.g = g
	return v
}

func (v *volume) at(off int64, n uint64) []byte {
	v.t.Helper()
	buf := make([]byte, n)
	if _, err := v.f.ReadAt(buf, off); err != nil {
		v.t.Fatalf("reading %d bytes at %d: %v", n, off, err)
	}
	return buf
}

func (v *volume) fatEntry(cluster uint32) uint32 {
	return binary.LittleEndian.Uint32(v.at(v.g.fatEntryOffset(cluster), 4))
}

// rootEntry returns the first root-directory entry of the given type.
func (v *volume) rootEntry(entryType byte) []byte {
	v.t.Helper()
	root := v.at(v.g.clusterOffset(v.g.rootCluster), v.g.bytesPerCluster())
	for at := 0; at+exFATDirEntryBytes <= len(root); at += exFATDirEntryBytes {
		if root[at] == entryType {
			return root[at : at+exFATDirEntryBytes]
		}
	}
	v.t.Fatalf("no 0x%02X entry in the root directory", entryType)
	return nil
}

// TestFormatExFATRoundTripsThroughInspect is the headline proof: a volume this
// package wrote is one this package recognises and can name, across the whole
// cluster-size ladder.
func TestFormatExFATRoundTripsThroughInspect(t *testing.T) {
	for _, tc := range []struct {
		name              string
		size              int64
		wantSectorsPerClu uint32
	}{
		{"8 MiB, 4 KiB clusters", 8 << 20, 8},
		{"1 GiB, 32 KiB clusters", 1 << 30, 64},
		{"64 GiB, 128 KiB clusters", 64 << 30, 256},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := formatted(t, tc.size, "BETAMIN")

			got, err := Inspect(v.path)
			if err != nil {
				t.Fatalf("Inspect: %v", err)
			}
			if got.FS != ExFAT || got.Label != "BETAMIN" {
				t.Errorf("Inspect = %+v, want {FS:exfat Label:BETAMIN}", got)
			}
			if v.g.sectorsPerCluster != tc.wantSectorsPerClu {
				t.Errorf("sectors per cluster = %d, want %d", v.g.sectorsPerCluster, tc.wantSectorsPerClu)
			}
			// The cluster heap must fit inside the volume it claims to describe.
			end := uint64(v.g.clusterHeapOffset) + uint64(v.g.clusterCount)*uint64(v.g.sectorsPerCluster)
			if end > v.g.volumeLength {
				t.Errorf("cluster heap ends at sector %d, past the volume's %d", end, v.g.volumeLength)
			}
		})
	}
}

// specRollingChecksum is a second, independently-written implementation of
// the Microsoft exFAT specification's rolling checksum, transcribed directly
// from the spec's published pseudocode (section 3.4, "Main Boot Checksum
// Sub-region"):
//
//	UINT32 Checksum = 0;
//	for (i = 0; i < Length; i++) {
//	    if (i is one of the excluded byte offsets) continue;
//	    Checksum = ((Checksum << 31) | (Checksum >> 1)) + (UINT32)Data[i];
//	}
//
// exFATRollingChecksum (exfatformat.go) computes the same rotate with
// bits.RotateLeft32(sum, -1); this spells the rotate out with shifts instead,
// so the two do not share a rotate-direction (or any other) bug by
// construction. TestFormatExFATBootRegionValidates and
// TestFormatExFATUpcaseTableIsWhatItClaims use this as an independent oracle
// for the writer's checksums, rather than recomputing "want" with the same
// function that produced them — see TestExFATRollingChecksumMatchesTheSpecByHand
// for both implementations pinned against a value worked out by hand.
func specRollingChecksum(data []byte, skipped ...int) uint32 {
	var checksum uint32
	for i, b := range data {
		if slices.Contains(skipped, i) {
			continue
		}
		checksum = ((checksum << 31) | (checksum >> 1)) + uint32(b)
	}
	return checksum
}

// TestExFATRollingChecksumMatchesTheSpecByHand pins exFATRollingChecksum (the
// production implementation) and specRollingChecksum (this test file's
// independent oracle, see its doc comment) against values worked out by hand
// from the specification's pseudocode, so a rotate-direction or accumulation
// bug in either one fails here instead of only ever being checked against the
// other.
//
// Derivation for data = {0x01, 0x02, 0x03, 0x04}, no exclusions (rotr(x, 1)
// moves bit 0 into bit 31 and shifts every other bit right by one):
//
//	i=0: checksum = rotr(0x00000000, 1) + 0x01 = 0x00000000 + 1 = 0x00000001
//	i=1: checksum = rotr(0x00000001, 1) + 0x02 = 0x80000000 + 2 = 0x80000002
//	i=2: checksum = rotr(0x80000002, 1) + 0x03 = 0x40000001 + 3 = 0x40000004
//	i=3: checksum = rotr(0x40000004, 1) + 0x04 = 0x20000002 + 4 = 0x20000006
//
// Skipping index 1 (the boot region's VolumeFlags/PercentInUse exclusion
// pattern) instead:
//
//	i=0: checksum = rotr(0x00000000, 1) + 0x01 = 0x00000000 + 1 = 0x00000001
//	i=1: skipped
//	i=2: checksum = rotr(0x00000001, 1) + 0x03 = 0x80000000 + 3 = 0x80000003
//	i=3: checksum = rotr(0x80000003, 1) + 0x04 = 0xC0000001 + 4 = 0xC0000005
func TestExFATRollingChecksumMatchesTheSpecByHand(t *testing.T) {
	data := []byte{0x01, 0x02, 0x03, 0x04}

	for _, tc := range []struct {
		name    string
		skipped []int
		want    uint32
	}{
		{"no exclusions", nil, 0x20000006},
		{"skip index 1", []int{1}, 0xC0000005},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := exFATRollingChecksum(data, tc.skipped...); got != tc.want {
				t.Errorf("exFATRollingChecksum = %#08x, want %#08x", got, tc.want)
			}
			if got := specRollingChecksum(data, tc.skipped...); got != tc.want {
				t.Errorf("specRollingChecksum = %#08x, want %#08x", got, tc.want)
			}
		})
	}
}

// TestFormatExFATBootRegionValidates recomputes the boot checksum the way any
// driver does before trusting a volume, and checks the structural markers a
// driver looks for. A volume that fails this is one Linux and macOS refuse.
//
// It checks the writer's checksum against specRollingChecksum rather than
// exFATRollingChecksum — the function under test elsewhere in this file — so
// a bug in that function's rotate or accumulation would fail here rather than
// passing because both sides of the comparison share the bug.
func TestFormatExFATBootRegionValidates(t *testing.T) {
	const sector = exFATFormatSectorSize
	v := formatted(t, 8<<20, "BETAMIN")
	main := v.at(0, exFATBootRegionSectors*sector)

	want := specRollingChecksum(main[:exFATBootChecksumSector*sector],
		exFATOffVolumeFlags, exFATOffVolumeFlags+1, exFATOffPercentInUse)
	for at := 0; at < sector; at += 4 {
		got := binary.LittleEndian.Uint32(main[exFATBootChecksumSector*sector+at:])
		if got != want {
			t.Fatalf("checksum sector at +%d = %#08x, want %#08x repeated", at, got, want)
		}
	}

	for s := 1; s <= 8; s++ {
		if got := binary.LittleEndian.Uint32(main[(s+1)*sector-4:]); got != 0xAA550000 {
			t.Errorf("extended boot sector %d signature = %#08x, want 0xAA550000", s, got)
		}
	}

	// The backup region exists so a reader can recover from a damaged main one,
	// which only works if it is a faithful copy.
	backup := v.at(exFATBootRegionSectors*sector, exFATBootRegionSectors*sector)
	if !bytes.Equal(main, backup) {
		t.Error("the backup boot region is not a copy of the main one")
	}
}

// TestFormatExFATAllocationBitmapMatchesTheFAT pins the invariant a filesystem
// checker tests: every cluster the FAT chains reach is marked allocated, and no
// other cluster is. Disagreement here is how a filesystem hands out a cluster
// it is already using.
func TestFormatExFATAllocationBitmapMatchesTheFAT(t *testing.T) {
	v := formatted(t, 1<<30, "BETAMIN")

	bitmapEntry := v.rootEntry(exFATEntryBitmap)
	bitmapCluster := binary.LittleEndian.Uint32(bitmapEntry[20:])
	bitmapBytes := binary.LittleEndian.Uint64(bitmapEntry[24:])
	if want := uint64(v.g.clusterCount+7) / 8; bitmapBytes != want {
		t.Errorf("bitmap DataLength = %d, want %d (one bit per cluster)", bitmapBytes, want)
	}

	// Walk every chain the root directory points at, plus the root's own.
	chained := map[uint32]bool{}
	for _, start := range []uint32{
		bitmapCluster,
		binary.LittleEndian.Uint32(v.rootEntry(exFATEntryUpcase)[20:]),
		v.g.rootCluster,
	} {
		for cluster := start; cluster < 0xFFFFFFF8; cluster = v.fatEntry(cluster) {
			if chained[cluster] {
				t.Fatalf("cluster %d appears in two chains", cluster)
			}
			chained[cluster] = true
		}
	}

	bitmap := v.at(v.g.clusterOffset(bitmapCluster), bitmapBytes)
	for cluster := uint32(exFATFirstCluster); cluster < v.g.clusterCount+exFATFirstCluster; cluster++ {
		bit := cluster - exFATFirstCluster
		allocated := bitmap[bit/8]&(1<<(bit%8)) != 0
		if allocated != chained[cluster] {
			t.Fatalf("cluster %d: bitmap says allocated=%v, FAT chains say %v", cluster, allocated, chained[cluster])
		}
	}
}

// TestFormatExFATUpcaseTableIsWhatItClaims checks the two things a driver
// checks: the recorded checksum matches the bytes on disk, and the table
// decompresses to the full Basic Multilingual Plane. Linux rejects a table that
// covers less, whatever its checksum.
//
// The checksum comparison uses specRollingChecksum (an independent oracle, see
// its doc comment), not exFATRollingChecksum — the writer used the latter to
// produce the recorded value, so recomputing "got" with the same function
// would only prove the writer is self-consistent, not that the checksum
// algorithm itself is right.
func TestFormatExFATUpcaseTableIsWhatItClaims(t *testing.T) {
	v := formatted(t, 8<<20, "BETAMIN")

	entry := v.rootEntry(exFATEntryUpcase)
	table := v.at(v.g.clusterOffset(binary.LittleEndian.Uint32(entry[20:])), binary.LittleEndian.Uint64(entry[24:]))

	if got, want := specRollingChecksum(table), binary.LittleEndian.Uint32(entry[4:]); got != want {
		t.Errorf("up-case table checksum = %#08x, but the directory entry records %#08x", got, want)
	}

	upcase, covered := decompressUpcase(table)
	if covered != 0x10000 {
		t.Fatalf("up-case table covers %d characters, want the whole BMP (65536)", covered)
	}
	// The specification makes the ASCII mappings mandatory; the rest spot-checks
	// scripts that case-fold differently from ASCII.
	for _, tc := range []struct{ from, want rune }{
		{'a', 'A'}, {'z', 'Z'}, {'A', 'A'}, {'0', '0'}, {' ', ' '},
		{'é', 'É'}, {'ω', 'Ω'}, {'я', 'Я'},
	} {
		if got := upcase[tc.from]; got != uint16(tc.want) {
			t.Errorf("up-case of %q = %U, want %U", tc.from, rune(got), tc.want)
		}
	}
}

// decompressUpcase expands the specification's run-length encoding exactly as
// Linux's exfat driver does, returning the mapping and how many characters it
// covered.
func decompressUpcase(table []byte) (map[rune]uint16, int) {
	upcase := map[rune]uint16{}
	index := 0
	skip := false
	for at := 0; at+2 <= len(table) && index <= 0xFFFF; at += 2 {
		unit := binary.LittleEndian.Uint16(table[at:])
		switch {
		case skip:
			for range int(unit) {
				upcase[rune(index)] = uint16(index)
				index++
			}
			skip = false
		case unit == 0xFFFF:
			skip = true
		default:
			upcase[rune(index)] = unit
			index++
		}
	}
	return upcase, index
}

func TestFormatExFATRefusesAVolumeTooSmallToHold(t *testing.T) {
	if err := FormatExFAT(backingFile(t, 64*1024), "BETAMIN"); err == nil {
		t.Fatal("formatted a 64 KiB volume as exFAT")
	}
}

func TestFormatExFATRefusesAnOverlongLabel(t *testing.T) {
	if err := FormatExFAT(backingFile(t, 8<<20), "TWELVECHARSX"); err == nil {
		t.Fatal("accepted a 12-character volume label")
	}
}

// TestFormatExFATLiftsTheFAT32FileSizeCeiling records why exFAT is offered at
// all: FAT32 cannot describe a file of 4 GiB or more, and the volumes written
// here have the free space to hold one.
func TestFormatExFATLiftsTheFAT32FileSizeCeiling(t *testing.T) {
	v := formatted(t, 64<<30, "BETAMIN")

	free := (uint64(v.g.clusterCount) - 3) * v.g.bytesPerCluster()
	if free <= 4<<30 {
		t.Errorf("a 64 GiB exFAT volume has %d free bytes, too few for a file above FAT32's 4 GiB ceiling", free)
	}
}
