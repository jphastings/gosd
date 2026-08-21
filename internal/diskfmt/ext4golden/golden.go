package ext4golden

import _ "embed"

// Golden is one checked-in seed filesystem: the zstd-compressed bytes and
// the exact size they decompress to. GoSD ships two — see README.md for why
// one image cannot serve both jobs — and diskfmt's FormatEXT4/WriteEXT4 take
// a caller's choice between them rather than defaulting to either.
type Golden struct {
	// Name identifies this golden in errors and in provenance. It matches
	// its manifest's "variant" field and build/ext4-golden/build.sh's
	// argument ("data", "config").
	Name string

	// Compressed is the zstd-compressed image: gosd-apmv's
	// diskfmt.FormatEXT4 decompresses it and streams it onto a target block
	// device or image file, then stamps a fresh per-volume UUID and label
	// into the superblock.
	Compressed []byte

	// RawBytes is the decompressed size of Compressed, i.e. the exact size
	// of the ext4 filesystem it expands to. It must equal the matching
	// manifest's rawImage.sizeBytes; TestManifestsDescribeTheCheckedInAssets
	// asserts exactly that, so an asset regenerated without its constant —
	// or the other way round — fails rather than drifting.
	RawBytes int64
}

//go:embed golden.img.zst
var dataCompressed []byte

//go:embed config-golden.img.zst
var configCompressed []byte

// Data is the golden that seeds every volume GoSD grows: /data on an ext4
// image, and every emmc/disk volume an app formats. Its 128MiB journal is
// sized for the GROWN filesystem's whole life — a journal can never be
// resized after format — which is what puts the floor under its own 512MiB
// size, and it ships ^resize_inode,meta_bg to escape resize_inode's ~8TiB
// growth ceiling. See README.md.
var Data = Golden{Name: "data", Compressed: dataCompressed, RawBytes: 512 * 1024 * 1024}

// Config is the golden that seeds the config partition, which is fixed-size
// and in the common case never grows at all. Its journal is at the ext4
// minimum (1024 blocks = 4MiB) and its feature set is deliberately ordinary
// — resize_inode, not meta_bg — because a partition that can never approach
// meta_bg's ceiling is better served by the filesystem shape any host's
// e2fsck knows best. See README.md.
var Config = Golden{Name: "config", Compressed: configCompressed, RawBytes: 32 * 1024 * 1024}
