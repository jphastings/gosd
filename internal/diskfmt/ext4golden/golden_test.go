// Package ext4golden holds the checked-in ext4 "golden images" (see
// README.md) that gosd raw-copies onto block devices and image partitions.
// This file is the pure-Go, no-mount, no-Docker contract test for both
// assets: it pins the exact superblock bytes bean gosd-apmv's Inspect/Format
// implementation (which go:embeds them) is built against, without needing a
// Linux kernel to mount anything.
package ext4golden

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
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
	sbOffJnlBlocks       = 0x10C
	sbOffBlocksCountHi   = 0x150
	sbOffChecksumSeed    = 0x270

	sbMagic = 0xEF53

	wantBlockSize = 4096
)

// goldenContract is what one checked-in asset must be, field by field. Every
// value here is a decision recorded in README.md, not an observation of
// whatever mke2fs happened to produce.
type goldenContract struct {
	golden Golden

	// manifestPath is the provenance file build.sh writes beside the asset.
	manifestPath string
	// assetPath is the compressed asset itself, read from disk (rather than
	// through the embed) so a regeneration that replaced the file without
	// rebuilding this package still fails.
	assetPath string

	featureCompat   uint32
	featureIncompat uint32
	featureRoCompat uint32

	// journalBytes is the parameter that most decides what a golden is FOR,
	// and the one no feature flag reveals: a journal can never be resized
	// after format, so the data golden's has to serve a grown multi-terabyte
	// filesystem's whole life while the config golden's sits at ext4's
	// minimum. Read out of the superblock's own journal-inode backup (see
	// journalSizeBytes).
	journalBytes uint64

	uuid string
}

var contracts = []goldenContract{
	{
		golden:        Data,
		manifestPath:  "manifest.json",
		assetPath:     "golden.img.zst",
		featureCompat: 0x2C, // has_journal | ext_attr | dir_index
		// filetype | meta_bg | extent | 64bit | flex_bg |
		// metadata_csum_seed. meta_bg is what makes online growth past ~8TiB
		// possible; the classic alternative, resize_inode (a compat-namespace
		// flag, so it appears in featureCompat rather than here), is
		// deliberately disabled at format time — the two are mutually
		// exclusive. See README.md's "Why meta_bg, not resize_inode".
		featureIncompat: 0x22D2,
		// sparse_super | large_file | huge_file | dir_nlink | extra_isize |
		// metadata_csum.
		featureRoCompat: 0x46B,
		journalBytes:    128 << 20,
		uuid:            "4c1a41c8-20b8-4c50-8399-7fae324e8398",
	},
	{
		golden:       Config,
		manifestPath: "config-manifest.json",
		assetPath:    "config-golden.img.zst",
		// has_journal | ext_attr | resize_inode | dir_index. resize_inode is
		// the one deliberate difference from the data golden, and it differs
		// in both directions: present here, absent there.
		featureCompat: 0x3C,
		// The data golden's set minus meta_bg: filetype | extent | 64bit |
		// flex_bg | metadata_csum_seed.
		featureIncompat: 0x22C2,
		featureRoCompat: 0x46B,
		// ext4's minimum journal, 1024 blocks at this block size.
		journalBytes: 4 << 20,
		uuid:         "d33ae914-c738-4bea-ba4d-99fe3c1bf25d",
	},
}

func TestGoldenImageSuperblocks(t *testing.T) {
	for _, c := range contracts {
		t.Run(c.golden.Name, func(t *testing.T) {
			raw := decompress(t, mustReadFile(t, c.assetPath))
			if int64(len(raw)) != c.golden.RawBytes {
				t.Fatalf("decompressed image is %d bytes, want %d (%d MiB)", len(raw), c.golden.RawBytes, c.golden.RawBytes/(1<<20))
			}

			const sbStart = 1024
			sb := raw[sbStart : sbStart+1024]

			if magic := binary.LittleEndian.Uint16(sb[sbOffMagic:]); magic != sbMagic {
				t.Errorf("magic = 0x%04X, want 0x%04X", magic, sbMagic)
			}

			if got := binary.LittleEndian.Uint32(sb[sbOffFeatureCompat:]); got != c.featureCompat {
				t.Errorf("feature_compat = 0x%X, want 0x%X", got, c.featureCompat)
			}
			if got := binary.LittleEndian.Uint32(sb[sbOffFeatureIncompat:]); got != c.featureIncompat {
				t.Errorf("feature_incompat = 0x%X, want 0x%X", got, c.featureIncompat)
			}
			if got := binary.LittleEndian.Uint32(sb[sbOffFeatureRoCompat:]); got != c.featureRoCompat {
				t.Errorf("feature_ro_compat = 0x%X, want 0x%X", got, c.featureRoCompat)
			}

			logBlockSize := binary.LittleEndian.Uint32(sb[sbOffLogBlockSize:])
			if blockSize := 1024 << logBlockSize; blockSize != wantBlockSize {
				t.Errorf("block size = %d, want %d", blockSize, wantBlockSize)
			}

			lo := binary.LittleEndian.Uint32(sb[sbOffBlocksCountLo:])
			hi := binary.LittleEndian.Uint32(sb[sbOffBlocksCountHi:])
			wantBlockCount := uint64(c.golden.RawBytes) / wantBlockSize
			if blockCount := uint64(hi)<<32 | uint64(lo); blockCount != wantBlockCount {
				t.Errorf("block count = %d, want %d", blockCount, wantBlockCount)
			}

			if got := journalSizeBytes(sb); got != c.journalBytes {
				t.Errorf("journal size = %d bytes (%d MiB), want %d (%d MiB)", got, got/(1<<20), c.journalBytes, c.journalBytes/(1<<20))
			}

			if label := sb[sbOffVolumeName : sbOffVolumeName+16]; !bytes.Equal(label, make([]byte, 16)) {
				t.Errorf("volume label = %q, want empty (all zero)", label)
			}

			if uuid := formatUUID(sb[sbOffUUID : sbOffUUID+16]); uuid != c.uuid {
				t.Errorf("UUID = %s, want %s", uuid, c.uuid)
			}

			// metadata_csum_seed (asserted above via feature_incompat) makes
			// this field meaningful: it's crc32c(~0, the original UUID),
			// computed once at format time and reused by every metadata
			// checksum in the filesystem -- see README.md's
			// "metadata_csum_seed, verified". We only assert it was actually
			// populated, not its exact value, since that's an implementation
			// detail of e2fsprogs' crc32c, not this asset's contract.
			if seed := binary.LittleEndian.Uint32(sb[sbOffChecksumSeed:]); seed == 0 {
				t.Error("checksum seed is 0, want a populated crc32c(~0, uuid) value")
			}
		})
	}
}

// TestTheTwoGoldensAreDistinct guards the mistake the two-golden split exists
// to make impossible: a copy-paste regeneration shipping the same filesystem
// twice under two names would leave every other assertion in this file
// passing.
func TestTheTwoGoldensAreDistinct(t *testing.T) {
	if bytes.Equal(Data.Compressed, Config.Compressed) {
		t.Fatal("the data and config goldens are byte-identical; one was regenerated with the other's parameters")
	}
	if Data.RawBytes == Config.RawBytes {
		t.Errorf("both goldens are %d bytes; the config golden exists precisely because the data golden's size has a 128MiB-journal floor under it", Data.RawBytes)
	}
}

// manifest is the subset of build.sh's provenance output this test verifies.
type manifest struct {
	Variant string `json:"variant"`
	MkE2fs  struct {
		GoldenSizeMiB  int64 `json:"goldenSizeMiB"`
		JournalSizeMiB int64 `json:"journalSizeMiB"`
	} `json:"mke2fs"`
	RawImage struct {
		SizeBytes int64  `json:"sizeBytes"`
		SHA256    string `json:"sha256"`
	} `json:"rawImage"`
	CompressedAsset struct {
		SizeBytes int64  `json:"sizeBytes"`
		SHA256    string `json:"sha256"`
		Path      string `json:"path"`
	} `json:"compressedAsset"`
}

// TestManifestsDescribeTheCheckedInAssets is the drift pin between an asset,
// the Golden constant that says how big it is, and the provenance file that
// claims to record both. Each of the three can be edited without the others,
// and a mismatch is invisible until a device formats a partition — so all
// three are compared here, against bytes read off disk rather than through
// the embed.
func TestManifestsDescribeTheCheckedInAssets(t *testing.T) {
	for _, c := range contracts {
		t.Run(c.golden.Name, func(t *testing.T) {
			var m manifest
			if err := json.Unmarshal(mustReadFile(t, c.manifestPath), &m); err != nil {
				t.Fatalf("parsing %s: %v", c.manifestPath, err)
			}

			// The data golden's manifest predates build.sh writing a
			// variant at all, so an empty one is the older format rather
			// than a wrong one; a populated one still has to agree.
			if m.Variant != "" && m.Variant != c.golden.Name {
				t.Errorf("%s describes variant %q, want %q", c.manifestPath, m.Variant, c.golden.Name)
			}
			if m.CompressedAsset.Path != c.assetPath {
				t.Errorf("%s describes asset %q, want %q", c.manifestPath, m.CompressedAsset.Path, c.assetPath)
			}

			compressed := mustReadFile(t, c.assetPath)
			if int64(len(compressed)) != m.CompressedAsset.SizeBytes {
				t.Errorf("%s is %d bytes, but %s records %d", c.assetPath, len(compressed), c.manifestPath, m.CompressedAsset.SizeBytes)
			}
			if got := sha256Hex(compressed); got != m.CompressedAsset.SHA256 {
				t.Errorf("%s sha256 is %s, but %s records %s", c.assetPath, got, c.manifestPath, m.CompressedAsset.SHA256)
			}
			if !bytes.Equal(compressed, c.golden.Compressed) {
				t.Errorf("the embedded %s golden differs from %s on disk", c.golden.Name, c.assetPath)
			}

			raw := decompress(t, compressed)
			if int64(len(raw)) != m.RawImage.SizeBytes {
				t.Errorf("%s decompresses to %d bytes, but %s records %d", c.assetPath, len(raw), c.manifestPath, m.RawImage.SizeBytes)
			}
			if got := sha256Hex(raw); got != m.RawImage.SHA256 {
				t.Errorf("decompressed %s sha256 is %s, but %s records %s", c.assetPath, got, c.manifestPath, m.RawImage.SHA256)
			}
			if m.RawImage.SizeBytes != c.golden.RawBytes {
				t.Errorf("%s records a %d-byte filesystem, but the Golden constant says %d", c.manifestPath, m.RawImage.SizeBytes, c.golden.RawBytes)
			}
			if want := c.golden.RawBytes / (1 << 20); m.MkE2fs.GoldenSizeMiB != want {
				t.Errorf("%s records goldenSizeMiB %d, want %d", c.manifestPath, m.MkE2fs.GoldenSizeMiB, want)
			}
			if want := int64(c.journalBytes / (1 << 20)); m.MkE2fs.JournalSizeMiB != want {
				t.Errorf("%s records journalSizeMiB %d, but the superblock's journal is %d MiB", c.manifestPath, m.MkE2fs.JournalSizeMiB, want)
			}
		})
	}
}

// journalSizeBytes reads the journal's size out of the superblock's
// s_jnl_blocks[17] array, which backs up the journal inode's own i_block[]
// plus its size: entries 0-14 are i_block[0..14], 15 is i_size_high and 16 is
// i_size_lo (kernel.org's filesystems/ext4/super.html). It is the only place
// a journal's size can be read without walking the inode table.
func journalSizeBytes(sb []byte) uint64 {
	lo := binary.LittleEndian.Uint32(sb[sbOffJnlBlocks+16*4:])
	hi := binary.LittleEndian.Uint32(sb[sbOffJnlBlocks+15*4:])
	return uint64(hi)<<32 | uint64(lo)
}

func decompress(t *testing.T, compressed []byte) []byte {
	t.Helper()
	zr, err := zstd.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatalf("creating zstd reader: %v", err)
	}
	defer zr.Close()
	raw, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("decompressing: %v", err)
	}
	return raw
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return data
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// formatUUID renders a 16-byte RFC 4122 UUID as the canonical
// 8-4-4-4-12 hex string.
func formatUUID(b []byte) string {
	s := hex.EncodeToString(b)
	return s[0:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:32]
}
