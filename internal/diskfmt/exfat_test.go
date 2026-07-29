package diskfmt

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf16"
)

// exFATFixture is a hand-built exFAT volume: a boot sector with a chosen
// geometry, a FAT, and a cluster heap the test scribbles directory entries
// into. It is written from the specification rather than from FormatExFAT, so
// the reader is checked against an independent oracle.
type exFATFixture struct {
	data []byte
	g    exFATGeometry
}

func newExFATFixture(t *testing.T) *exFATFixture {
	t.Helper()
	const (
		sectorShift  = 9
		clusterShift = 3 // 8 sectors = 4 KiB clusters
	)
	g := exFATGeometry{
		fatOffset:         32,
		fatLength:         8,
		clusterHeapOffset: 64,
		clusterCount:      64,
		rootCluster:       2,
		bytesPerSector:    1 << sectorShift,
		sectorsPerCluster: 1 << clusterShift,
	}
	g.volumeLength = uint64(g.clusterHeapOffset) + uint64(g.clusterCount)*uint64(g.sectorsPerCluster)

	f := &exFATFixture{data: make([]byte, g.volumeLength*uint64(g.bytesPerSector)), g: g}
	boot := f.data[:512]
	copy(boot[0:3], []byte{0xEB, 0x76, 0x90})
	copy(boot[3:11], exFATMagic)
	binary.LittleEndian.PutUint64(boot[exFATOffVolumeLength:], g.volumeLength)
	binary.LittleEndian.PutUint32(boot[exFATOffFatOffset:], g.fatOffset)
	binary.LittleEndian.PutUint32(boot[exFATOffFatLength:], g.fatLength)
	binary.LittleEndian.PutUint32(boot[exFATOffClusterHeapOffset:], g.clusterHeapOffset)
	binary.LittleEndian.PutUint32(boot[exFATOffClusterCount:], g.clusterCount)
	binary.LittleEndian.PutUint32(boot[exFATOffRootCluster:], g.rootCluster)
	binary.LittleEndian.PutUint16(boot[exFATOffRevision:], 0x0100)
	boot[exFATOffSectorShift] = sectorShift
	boot[exFATOffClusterShift] = clusterShift
	boot[exFATOffNumberOfFats] = 1
	boot[exFATOffDriveSelect] = 0x80
	binary.LittleEndian.PutUint16(boot[exFATOffBootSignature:], 0xAA55)
	return f
}

// putLabel writes a 0x83 volume-label entry at a directory entry slot.
func (f *exFATFixture) putLabel(cluster uint32, slot int, label string) {
	at := f.g.clusterOffset(cluster) + int64(slot*exFATDirEntryBytes)
	entry := f.data[at : at+exFATDirEntryBytes]
	units := utf16.Encode([]rune(label))
	entry[0] = exFATEntryVolLabel
	entry[1] = byte(len(units))
	for i, u := range units {
		binary.LittleEndian.PutUint16(entry[2+2*i:], u)
	}
}

// link points a cluster's FAT entry at the next cluster in its chain.
func (f *exFATFixture) link(from, to uint32) {
	binary.LittleEndian.PutUint32(f.data[f.g.fatEntryOffset(from):], to)
}

func (f *exFATFixture) path(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "exfat.img")
	if err := os.WriteFile(path, f.data, 0o600); err != nil {
		t.Fatalf("writing exFAT fixture: %v", err)
	}
	return path
}

func TestInspectReadsAnExFATVolumeLabel(t *testing.T) {
	// The bench case: a KIOXIA NVMe that arrived exFAT-formatted. Reading its
	// label is what lets it be mounted instead of wiped.
	f := newExFATFixture(t)
	f.putLabel(f.g.rootCluster, 0, "BETAMIN")

	got, err := Inspect(f.path(t))
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if got.FS != ExFAT || got.Label != "BETAMIN" {
		t.Errorf("Inspect = %+v, want {FS:exfat Label:BETAMIN}", got)
	}
}

// TestInspectFindsALabelBeyondTheFirstRootCluster proves the root directory is
// followed as a FAT chain rather than assumed to be one cluster: an exFAT
// volume that has accumulated files has a multi-cluster root directory.
func TestInspectFindsALabelBeyondTheFirstRootCluster(t *testing.T) {
	f := newExFATFixture(t)
	// Fill the first root cluster with in-use file entries (0x85), so the walk
	// must not stop there, then chain it to a second cluster holding the label.
	entries := int(f.g.bytesPerCluster()) / exFATDirEntryBytes
	for slot := range entries {
		f.data[f.g.clusterOffset(f.g.rootCluster)+int64(slot*exFATDirEntryBytes)] = 0x85
	}
	f.link(f.g.rootCluster, f.g.rootCluster+1)
	f.link(f.g.rootCluster+1, exFATEndOfChain)
	f.putLabel(f.g.rootCluster+1, 0, "SECONDCLUS")

	got, err := Inspect(f.path(t))
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if got.Label != "SECONDCLUS" {
		t.Errorf("Inspect label = %q, want SECONDCLUS", got.Label)
	}
}

func TestInspectReadsAnUnlabelledExFATVolume(t *testing.T) {
	// No 0x83 entry at all: still exFAT, still mountable, just nameless — which
	// must read as an empty label rather than an error, so the volume is
	// refused (label mismatch) rather than mistaken for a broken filesystem.
	f := newExFATFixture(t)
	f.link(f.g.rootCluster, exFATEndOfChain)

	got, err := Inspect(f.path(t))
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if got.FS != ExFAT || got.Label != "" {
		t.Errorf("Inspect = %+v, want {FS:exfat Label:\"\"}", got)
	}
}

func TestInspectRefusesExFATWithUnusableGeometry(t *testing.T) {
	for _, tc := range []struct {
		name    string
		corrupt func(*exFATFixture)
	}{
		{"no boot signature", func(f *exFATFixture) {
			binary.LittleEndian.PutUint16(f.data[exFATOffBootSignature:], 0)
		}},
		{"impossible sector size", func(f *exFATFixture) { f.data[exFATOffSectorShift] = 20 }},
		{"root directory outside the cluster heap", func(f *exFATFixture) {
			binary.LittleEndian.PutUint32(f.data[exFATOffRootCluster:], 9999)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newExFATFixture(t)
			f.putLabel(f.g.rootCluster, 0, "BETAMIN")
			tc.corrupt(f)

			got, err := Inspect(f.path(t))
			if err != nil {
				t.Fatalf("Inspect: %v", err)
			}
			// Named so a refusal can be specific, but never claimed mountable.
			if got.FS != "" || got.OtherFS != "exFAT" {
				t.Errorf("Inspect = %+v, want it named as unreadable exFAT", got)
			}
		})
	}
}

func TestDecodeExFATLabelTrimsPadding(t *testing.T) {
	entry := make([]byte, exFATDirEntryBytes)
	entry[0] = exFATEntryVolLabel
	entry[1] = 11
	for i, u := range utf16.Encode([]rune("BETAMIN    ")) {
		binary.LittleEndian.PutUint16(entry[2+2*i:], u)
	}

	if got := decodeExFATLabel(entry); got != "BETAMIN" {
		t.Errorf("decodeExFATLabel = %q, want BETAMIN", got)
	}
}

func TestIsExFATNeedsTheMagicAtTheRightOffset(t *testing.T) {
	if isExFAT(append([]byte{0xEB, 0x76}, exFATMagic...)) {
		t.Error("accepted the magic at the wrong offset")
	}
	if !isExFAT(append([]byte{0xEB, 0x76, 0x90}, exFATMagic...)) {
		t.Error("rejected the magic at offset 3")
	}
	if isExFAT(bytes.Repeat([]byte{0}, 32)) {
		t.Error("accepted a zeroed sector as exFAT")
	}
}
