package ext4golden

import _ "embed"

// Compressed is the zstd-compressed ext4 golden image (see README.md):
// gosd-apmv's diskfmt.FormatEXT4 decompresses it and streams it onto a
// target block device or image file, then stamps a fresh per-volume UUID
// and label into the superblock.
//
//go:embed golden.img.zst
var Compressed []byte

// RawBytes is the decompressed size of Compressed, i.e. the exact size of
// the ext4 filesystem golden.img.zst expands to. It must equal
// manifest.json's rawImage.sizeBytes and golden_test.go's wantBlockCount *
// wantBlockSize; a mismatch means the asset and this constant have drifted.
const RawBytes = 512 * 1024 * 1024
