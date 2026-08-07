package diskfmt

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// ext4 on-disk constants, all from kernel.org's ext4 on-disk format
// documentation (filesystems/ext4/super.html) and cross-checked against
// internal/diskfmt/ext4golden/golden_test.go, the pure-Go contract test on
// the checked-in golden image these offsets are read from and written to.
const (
	// ext4SuperblockOffset is the superblock's fixed byte offset from the
	// start of the volume, whatever the block size.
	ext4SuperblockOffset = 1024
	// ext4SuperblockSize is enough of the superblock to read every field
	// this package uses and to run its checksum over (the checksum field
	// itself, at ext4SuperblockChecksum, is the last 4 bytes of it).
	ext4SuperblockSize = 1024

	// Superblock field offsets, relative to the superblock's own start.
	ext4SuperblockOffMagic           = 0x038
	ext4SuperblockOffBlocksCountLo   = 0x004
	ext4SuperblockOffLogBlockSize    = 0x018
	ext4SuperblockOffBlocksPerGroup  = 0x020
	ext4SuperblockOffFeatureCompat   = 0x05C
	ext4SuperblockOffFeatureIncompat = 0x060
	ext4SuperblockOffFeatureRoCompat = 0x064
	ext4SuperblockOffUUID            = 0x068
	ext4SuperblockOffVolumeName      = 0x078
	ext4SuperblockOffBlocksCountHi   = 0x150
	ext4SuperblockOffChecksumSeed    = 0x270
	ext4SuperblockOffChecksum        = 0x3FC

	ext4Magic = 0xEF53

	// ext4LabelBytes is the fixed width of the s_volume_name field: ext4
	// labels are raw bytes, NUL-padded, with no room for a terminator when
	// all 16 are used.
	ext4LabelBytes = 16
)

// Known ext4 feature bits this package understands, named where used.
// Sources: kernel.org's ext4 on-disk format docs and e2fsprogs'
// ext2fs/ext2_fs.h. Bits not listed are ones this package has never been
// taught to interpret.
const (
	// ext4IncompatRecover ("needs recovery") is not a format-shape feature
	// like the others below — it is a transient flag the KERNEL sets when it
	// opens the journal for writing and clears only on a clean unmount. GoSD
	// boards never cleanly unmount (gosd-init has no shutdown path — see
	// CLAUDE.md's "gosd-init has no interactive surface"), so this bit is
	// set on essentially every real-world reboot, and deliberately after a
	// hard power cut or qemu kill: it is the expected, common case, not an
	// exceptional one. Every field this package's Inspect/parseEXT4Superblock
	// read — feature bits, block count, label, UUID — is written with a
	// direct fsync/syncfs by Format/Grow/EstablishMarker (see
	// internal/blockmount's crash-ordering argument), never left pending in
	// the journal, so a pending replay does not make any of them
	// untrustworthy to read here. The replay itself happens inside the
	// kernel's own Mount call that follows Inspect, before anything reads
	// file *data*. Treating this bit as "unknown" refused adoption after
	// almost every real reboot until CI's qemu-disk-ext4 job (gosd-ucgr)
	// caught it by actually rebooting past a hard kill, which none of this
	// package's earlier device-file-only tests did.
	ext4IncompatRecover = 0x0004

	ext4IncompatFiletype         = 0x0002
	ext4IncompatMetaBG           = 0x0010
	ext4IncompatExtents          = 0x0040
	ext4Incompat64Bit            = 0x0080
	ext4IncompatFlexBG           = 0x0200
	ext4IncompatMetadataCsumSeed = 0x2000

	ext4RoCompatSparseSuper  = 0x0001
	ext4RoCompatMetadataCsum = 0x0400
)

// ext4KnownIncompat is the set of INCOMPAT feature bits GoSD's ext4 code
// tolerates reading: the on-disk feature set the golden image is built with
// (see internal/diskfmt/ext4golden/manifest.json and README.md), which is
// also the only feature set FormatEXT4 ever produces, plus ext4IncompatRecover
// (see its own doc comment — a runtime flag, not a format feature). ext4's
// own definition of "incompat" is that a reader which doesn't recognise one
// of these bits must not attempt to interpret the filesystem at all — so an
// incompat bit outside this set is refused rather than guessed at (see
// parseEXT4Superblock), instead of risking a future or foreign ext4 volume
// being silently misdescribed as something GoSD can safely adopt or format
// over.
const ext4KnownIncompat = ext4IncompatRecover | ext4IncompatFiletype | ext4IncompatMetaBG | ext4IncompatExtents |
	ext4Incompat64Bit | ext4IncompatFlexBG | ext4IncompatMetadataCsumSeed

// errNotEXT4 is wrapped into every reason parseEXT4Superblock refuses a
// buffer, mirroring errNotExFAT.
var errNotEXT4 = errors.New("not an ext4 volume")

// ext4Superblock is the subset of an ext4 superblock's fields this package
// reads or needs to recompute a checksum over.
type ext4Superblock struct {
	blockSize       uint32
	blocksPerGroup  uint32
	blockCount      uint64
	featureIncompat uint32
	featureRoCompat uint32
	label           string
	uuid            [16]byte
}

// isEXT4 reports whether head's bytes carry an ext4 superblock's magic
// number at its fixed offset. head must be at least ext4SuperblockOffset +
// ext4SuperblockSize bytes for this to find it; diskfmt.Inspect's
// blankProbeBytes (1 MiB) comfortably covers it.
func isEXT4(head []byte) bool {
	if len(head) < ext4SuperblockOffset+ext4SuperblockSize {
		return false
	}
	sb := head[ext4SuperblockOffset : ext4SuperblockOffset+ext4SuperblockSize]
	return binary.LittleEndian.Uint16(sb[ext4SuperblockOffMagic:]) == ext4Magic
}

// parseEXT4Superblock reads and validates an ext4SuperblockSize-byte
// superblock. It refuses — rather than guesses at — a superblock whose
// INCOMPAT feature bits include any this package does not understand; see
// ext4KnownIncompat.
func parseEXT4Superblock(sb []byte) (ext4Superblock, error) {
	if len(sb) < ext4SuperblockSize {
		return ext4Superblock{}, fmt.Errorf("%w: superblock is only %d bytes", errNotEXT4, len(sb))
	}
	if magic := binary.LittleEndian.Uint16(sb[ext4SuperblockOffMagic:]); magic != ext4Magic {
		return ext4Superblock{}, fmt.Errorf("%w: magic is 0x%04X, want 0x%04X", errNotEXT4, magic, ext4Magic)
	}

	incompat := binary.LittleEndian.Uint32(sb[ext4SuperblockOffFeatureIncompat:])
	if unknown := incompat &^ ext4KnownIncompat; unknown != 0 {
		return ext4Superblock{}, fmt.Errorf(
			"this ext4 volume uses incompat feature bits 0x%X that GoSD's ext4 support does not understand (known bits: 0x%X); refusing to read it rather than risk misinterpreting — or, at format time, corrupting — a filesystem shaped by features this code was never taught",
			unknown, ext4KnownIncompat)
	}

	logBlockSize := binary.LittleEndian.Uint32(sb[ext4SuperblockOffLogBlockSize:])
	lo := binary.LittleEndian.Uint32(sb[ext4SuperblockOffBlocksCountLo:])
	hi := binary.LittleEndian.Uint32(sb[ext4SuperblockOffBlocksCountHi:])

	var uuid [16]byte
	copy(uuid[:], sb[ext4SuperblockOffUUID:ext4SuperblockOffUUID+16])

	return ext4Superblock{
		blockSize:       1024 << logBlockSize,
		blocksPerGroup:  binary.LittleEndian.Uint32(sb[ext4SuperblockOffBlocksPerGroup:]),
		blockCount:      uint64(hi)<<32 | uint64(lo),
		featureIncompat: incompat,
		featureRoCompat: binary.LittleEndian.Uint32(sb[ext4SuperblockOffFeatureRoCompat:]),
		label:           trimEXT4Label(sb[ext4SuperblockOffVolumeName : ext4SuperblockOffVolumeName+ext4LabelBytes]),
		uuid:            uuid,
	}, nil
}

// trimEXT4Label drops the trailing NUL padding s_volume_name is stored with.
// Unlike FAT's label field, ext4's is not space-padded.
func trimEXT4Label(label []byte) string {
	return strings.TrimRight(string(label), "\x00")
}

// formatEXT4UUID renders a 16-byte ext4 UUID as the canonical RFC 4122
// 8-4-4-4-12 hex string, the form tune2fs/blkid print.
func formatEXT4UUID(uuid [16]byte) string {
	s := hex.EncodeToString(uuid[:])
	return s[0:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:32]
}

// inspectEXT4 reads the label and UUID of the ext4 volume on devicePath,
// whose leading bytes are head. An error here is a hard refusal, not a "not
// ext4" result — isEXT4 has already confirmed the magic matches, so a
// failure past that point (most likely unknown incompat feature bits) must
// not be swallowed into Contents{OtherFS: ...} the way an unreadable exFAT
// volume is: this filesystem cannot be safely described at all.
func inspectEXT4(devicePath string, head []byte) (Contents, error) {
	sb, err := parseEXT4Superblock(head[ext4SuperblockOffset : ext4SuperblockOffset+ext4SuperblockSize])
	if err != nil {
		return Contents{}, fmt.Errorf("reading the ext4 superblock on %s failed: %w", devicePath, err)
	}
	return Contents{FS: EXT4, Label: sb.label, UUID: formatEXT4UUID(sb.uuid)}, nil
}
