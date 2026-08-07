package diskfmt

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"testing"

	"github.com/jphastings/gosd/internal/diskfmt/ext4golden"
	"github.com/klauspost/compress/zstd"
)

// decompressedGolden returns the raw, decompressed bytes of the checked-in
// ext4 golden image — the same asset FormatEXT4 embeds and streams.
func decompressedGolden(t *testing.T) []byte {
	t.Helper()
	zr, err := zstd.NewReader(bytes.NewReader(ext4golden.Compressed))
	if err != nil {
		t.Fatalf("creating zstd reader: %v", err)
	}
	defer zr.Close()
	raw, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("decompressing the golden image: %v", err)
	}
	if int64(len(raw)) != ext4golden.RawBytes {
		t.Fatalf("decompressed golden image is %d bytes, want %d", len(raw), ext4golden.RawBytes)
	}
	return raw
}

// TestEXT4ChecksumMatchesGoldenSuperblock is what makes ext4Checksum
// trustworthy without needing Docker: mke2fs wrote the golden image's own
// primary superblock checksum with real e2fsprogs, so recomputing it with
// this package's implementation and comparing is a direct check against
// ground truth, not just internal self-consistency.
func TestEXT4ChecksumMatchesGoldenSuperblock(t *testing.T) {
	raw := decompressedGolden(t)
	sb := raw[ext4SuperblockOffset : ext4SuperblockOffset+ext4SuperblockSize]

	want := binary.LittleEndian.Uint32(sb[ext4SuperblockOffChecksum:])
	got := ext4Checksum(0xFFFFFFFF, sb[:ext4SuperblockOffChecksum])
	if got != want {
		t.Errorf("ext4Checksum of the golden superblock = 0x%08X, want 0x%08X (mke2fs's own value)", got, want)
	}

	// The checksum seed is the same crc32c(seed, data) function applied to
	// the original UUID (kernel.org's documented definition) — checking it
	// too pins that this isn't a coincidental match on one input.
	wantSeed := binary.LittleEndian.Uint32(sb[ext4SuperblockOffChecksumSeed:])
	gotSeed := ext4Checksum(0xFFFFFFFF, sb[ext4SuperblockOffUUID:ext4SuperblockOffUUID+16])
	if gotSeed != wantSeed {
		t.Errorf("ext4Checksum of the golden UUID = 0x%08X, want the stored checksum seed 0x%08X", gotSeed, wantSeed)
	}
}

// TestEXT4BackupSuperblockOffsetsMatchTheGolden pins the sparse_super group
// list this package computes against the two backup copies mke2fs actually
// wrote into the 512 MiB, 128 MiB-per-group golden image: groups 1 and 3,
// not group 2.
func TestEXT4BackupSuperblockOffsetsMatchTheGolden(t *testing.T) {
	raw := decompressedGolden(t)
	sb, err := parseEXT4Superblock(raw[ext4SuperblockOffset : ext4SuperblockOffset+ext4SuperblockSize])
	if err != nil {
		t.Fatalf("parseEXT4Superblock: %v", err)
	}

	offsets, err := ext4BackupSuperblockOffsets(sb)
	if err != nil {
		t.Fatalf("ext4BackupSuperblockOffsets: %v", err)
	}

	want := []int64{128 << 20, 384 << 20} // groups 1 and 3, 128 MiB apart
	if len(offsets) != len(want) {
		t.Fatalf("backup offsets = %v, want %v", offsets, want)
	}
	for i, off := range offsets {
		if off != want[i] {
			t.Errorf("backup offset[%d] = %d, want %d", i, off, want[i])
		}
		if magic := binary.LittleEndian.Uint16(raw[off+ext4SuperblockOffMagic:]); magic != ext4Magic {
			t.Errorf("no ext4 magic at claimed backup offset %d (group %d)", off, i)
		}
	}
}

func TestIsEXT4(t *testing.T) {
	raw := decompressedGolden(t)
	if !isEXT4(raw) {
		t.Error("isEXT4(golden image) = false, want true")
	}

	blank := make([]byte, 2048)
	if isEXT4(blank) {
		t.Error("isEXT4(all-zero buffer) = true, want false")
	}

	corrupt := append([]byte(nil), raw[:2048]...)
	binary.LittleEndian.PutUint16(corrupt[ext4SuperblockOffset+ext4SuperblockOffMagic:], 0x1234)
	if isEXT4(corrupt) {
		t.Error("isEXT4 with a scrambled magic = true, want false")
	}
}

// TestInspectRefusesUnknownIncompatFeatures pins the bean's core safety
// requirement: an ext4 volume whose INCOMPAT bits include one this package
// was never taught is refused with an error, not silently reported as an
// adoptable ext4 filesystem (which is what would happen if unknown features
// were ignored) or misreported as blank/foreign.
func TestInspectRefusesUnknownIncompatFeatures(t *testing.T) {
	head := ext4HeadOnlyBackingFile(t)

	// Set a bit outside ext4KnownIncompat (e.g. the "encrypt" bit, 0x10000).
	setEXT4IncompatBit(t, head, 0x10000)

	_, err := Inspect(head)
	if err == nil {
		t.Fatal("Inspect with an unknown incompat feature bit = nil error, want a refusal")
	}
}

// setEXT4IncompatBit ORs bit into the backing file's ext4 feature_incompat
// field, in place.
func setEXT4IncompatBit(t *testing.T, path string, bit uint32) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("opening to corrupt: %v", err)
	}
	var cur [4]byte
	if _, err := f.ReadAt(cur[:], ext4SuperblockOffset+ext4SuperblockOffFeatureIncompat); err != nil {
		t.Fatalf("reading feature_incompat: %v", err)
	}
	binary.LittleEndian.PutUint32(cur[:], binary.LittleEndian.Uint32(cur[:])|bit)
	if _, err := f.WriteAt(cur[:], ext4SuperblockOffset+ext4SuperblockOffFeatureIncompat); err != nil {
		t.Fatalf("writing feature_incompat: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("closing: %v", err)
	}
}

// TestInspectToleratesRecoverFlag pins the fix for the bug CI's
// qemu-disk-ext4 job (gosd-ucgr) found by actually rebooting past a hard
// qemu kill: the kernel sets INCOMPAT_RECOVER (0x0004, "needs journal
// replay") on essentially every real reboot — GoSD boards never cleanly
// unmount — so treating it as an unknown feature refused adoption after
// almost every boot, not just a crash. Inspect must read straight through
// it, the same as it would for a pristine volume.
func TestInspectToleratesRecoverFlag(t *testing.T) {
	head := ext4HeadOnlyBackingFile(t)
	setEXT4IncompatBit(t, head, ext4IncompatRecover)

	got, err := Inspect(head)
	if err != nil {
		t.Fatalf("Inspect with only INCOMPAT_RECOVER set: %v", err)
	}
	if got.FS != EXT4 {
		t.Errorf("Inspect with only INCOMPAT_RECOVER set reported FS %q, want ext4", got.FS)
	}
}

// TestInspectCorruptMagicIsNotEXT4 is the "not ext4 at all" counterpart:
// scrambling only the magic must not be recognised as ext4 (it falls through
// to blank/foreign, same as any other unreadable content).
func TestInspectCorruptMagicIsNotEXT4(t *testing.T) {
	head := ext4HeadOnlyBackingFile(t)
	var zero [2]byte
	scribble(t, head, ext4SuperblockOffset+ext4SuperblockOffMagic, zero[:])

	got, err := Inspect(head)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if got.FS == EXT4 {
		t.Errorf("Inspect with a scrambled ext4 magic reported FS:ext4, want it unrecognised")
	}
}

// ext4HeadOnlyBackingFile writes just enough of the golden image's leading
// bytes (its own superblock, uncompressed) to a backing file the size of
// Inspect's own probe window — exercising the exact same Inspect/isEXT4/
// parseEXT4Superblock path a full device would, without the cost of writing
// out the whole 512 MiB golden image on every test.
func ext4HeadOnlyBackingFile(t *testing.T) string {
	t.Helper()
	raw := decompressedGolden(t)
	path := backingFile(t, blankProbeBytes)
	scribble(t, path, 0, raw[:blankProbeBytes])
	return path
}
