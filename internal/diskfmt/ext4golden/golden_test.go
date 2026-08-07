// Package ext4golden holds the checked-in ext4 "golden image" (see
// README.md) that gosd's disk package raw-copies onto internal drives and
// grows in place. This file is the pure-Go, no-mount, no-Docker contract
// test for golden.img.zst: it pins the exact superblock bytes bean
// gosd-apmv's Inspect/Format implementation (which go:embeds this asset)
// is built against, without needing a Linux kernel to mount anything.
package ext4golden

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"io"
	"os"
	"testing"

	"github.com/klauspost/compress/zstd"
)

// ext4 superblock field offsets, relative to the start of the 1024-byte
// superblock (which itself starts at byte offset 1024 in the device/image).
// Source: kernel.org's ext4 on-disk format docs,
// filesystems/ext4/super.html.
const (
	sbOffMagic           = 0x38
	sbOffBlocksCountLo   = 0x4
	sbOffLogBlockSize    = 0x18
	sbOffFeatureCompat   = 0x5C
	sbOffFeatureIncompat = 0x60
	sbOffFeatureRoCompat = 0x64
	sbOffUUID            = 0x68
	sbOffVolumeName      = 0x78
	sbOffBlocksCountHi   = 0x150
	sbOffChecksumSeed    = 0x270

	sbMagic = 0xEF53

	// Compat: has_journal(0x4) | ext_attr(0x8) | dir_index(0x20).
	wantFeatureCompat = 0x2C
	// Incompat: filetype(0x2) | meta_bg(0x10) | extent(0x40) | 64bit(0x80) |
	// flex_bg(0x200) | metadata_csum_seed(0x2000). meta_bg is what makes
	// online growth past ~8TiB possible; the classic alternative,
	// resize_inode (a compat-namespace flag, so it wouldn't show up here
	// anyway), is deliberately disabled at format time -- the two are
	// mutually exclusive. See README.md's "Why meta_bg, not resize_inode".
	wantFeatureIncompat = 0x22D2
	// Ro-compat: sparse_super(0x1) | large_file(0x2) | huge_file(0x8) |
	// dir_nlink(0x20) | extra_isize(0x40) | metadata_csum(0x400).
	wantFeatureRoCompat = 0x46B

	wantBlockSize  = 4096
	wantBlockCount = 512 * 1024 * 1024 / wantBlockSize

	wantUUID = "4c1a41c8-20b8-4c50-8399-7fae324e8398"
)

func TestGoldenImageSuperblock(t *testing.T) {
	compressed, err := os.ReadFile("golden.img.zst")
	if err != nil {
		t.Fatalf("reading golden.img.zst: %v", err)
	}

	zr, err := zstd.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatalf("creating zstd reader: %v", err)
	}
	defer zr.Close()

	raw, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("decompressing golden.img.zst: %v", err)
	}
	if len(raw) != wantBlockCount*wantBlockSize {
		t.Fatalf("decompressed image is %d bytes, want %d (%d MiB)", len(raw), wantBlockCount*wantBlockSize, wantBlockCount*wantBlockSize/1024/1024)
	}

	const sbStart = 1024
	sb := raw[sbStart : sbStart+1024]

	if magic := binary.LittleEndian.Uint16(sb[sbOffMagic:]); magic != sbMagic {
		t.Errorf("magic = 0x%04X, want 0x%04X", magic, sbMagic)
	}

	if got := binary.LittleEndian.Uint32(sb[sbOffFeatureCompat:]); got != wantFeatureCompat {
		t.Errorf("feature_compat = 0x%X, want 0x%X", got, wantFeatureCompat)
	}
	if got := binary.LittleEndian.Uint32(sb[sbOffFeatureIncompat:]); got != wantFeatureIncompat {
		t.Errorf("feature_incompat = 0x%X, want 0x%X", got, wantFeatureIncompat)
	}
	if got := binary.LittleEndian.Uint32(sb[sbOffFeatureRoCompat:]); got != wantFeatureRoCompat {
		t.Errorf("feature_ro_compat = 0x%X, want 0x%X", got, wantFeatureRoCompat)
	}

	logBlockSize := binary.LittleEndian.Uint32(sb[sbOffLogBlockSize:])
	if blockSize := 1024 << logBlockSize; blockSize != wantBlockSize {
		t.Errorf("block size = %d, want %d", blockSize, wantBlockSize)
	}

	lo := binary.LittleEndian.Uint32(sb[sbOffBlocksCountLo:])
	hi := binary.LittleEndian.Uint32(sb[sbOffBlocksCountHi:])
	if blockCount := uint64(hi)<<32 | uint64(lo); blockCount != wantBlockCount {
		t.Errorf("block count = %d, want %d", blockCount, wantBlockCount)
	}

	if label := sb[sbOffVolumeName : sbOffVolumeName+16]; !bytes.Equal(label, make([]byte, 16)) {
		t.Errorf("volume label = %q, want empty (all zero)", label)
	}

	if uuid := formatUUID(sb[sbOffUUID : sbOffUUID+16]); uuid != wantUUID {
		t.Errorf("UUID = %s, want %s", uuid, wantUUID)
	}

	// metadata_csum_seed (asserted above via feature_incompat) makes this
	// field meaningful: it's crc32c(~0, the original UUID), computed once at
	// format time and reused by every metadata checksum in the filesystem --
	// see README.md's "metadata_csum_seed, verified". We only assert it was
	// actually populated, not its exact value, since that's an
	// implementation detail of e2fsprogs' crc32c, not this asset's contract.
	if seed := binary.LittleEndian.Uint32(sb[sbOffChecksumSeed:]); seed == 0 {
		t.Error("checksum seed is 0, want a populated crc32c(~0, uuid) value")
	}
}

// formatUUID renders a 16-byte RFC 4122 UUID as the canonical
// 8-4-4-4-12 hex string.
func formatUUID(b []byte) string {
	s := hex.EncodeToString(b)
	return s[0:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:32]
}
