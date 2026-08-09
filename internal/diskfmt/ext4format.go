package diskfmt

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"

	"github.com/jphastings/gosd/internal/diskfmt/ext4golden"
	"github.com/klauspost/compress/zstd"
)

// ext4FormatChunkBytes bounds how much of the golden image is held in memory
// at once while it streams onto the target — RAM on the smallest GoSD board
// is scarce, so the whole 512 MiB golden is never buffered whole. It must
// stay large enough that every superblock copy the golden ships (the
// primary, plus each sparse_super backup) lands entirely inside a single
// chunk: ext4BackupSuperblockOffsets checks this at format time and refuses
// loudly rather than silently write a half-patched superblock if a future
// golden's block-group size ever broke that assumption.
const ext4FormatChunkBytes = 1 << 20 // 1 MiB

// ext4ChecksumTable is the reflected Castagnoli CRC-32C table, ext4's
// checksum polynomial (see ext4Checksum).
var ext4ChecksumTable = crc32.MakeTable(crc32.Castagnoli)

// ext4Checksum implements the crc32c(seed, data) function ext4's on-disk
// format is defined against (every metadata checksum, including the
// superblock's own): kernel.org's super.html describes s_checksum_seed as
// "crc32c(~0, orig_fs_uuid)", and fs/ext4/super.c's ext4_superblock_csum
// computes the superblock's checksum the same way, over the superblock's
// own bytes up to (not including) the checksum field.
//
// This is the reflected CRC-32C polynomial, but — unlike the "standard"
// CRC-32C used by e.g. iSCSI or SCTP — with no final complement of the
// running register: the seed IS the initial register value (typically ~0),
// and the returned value is the raw register after processing, not XORed
// with anything. Go's crc32.Update always complements its input and output
// (crc32.Checksum(data, tab) == Update(0, tab, data), the standard
// definition), so composing seed = ^wanted and complementing the result the
// same way exactly cancels that wrapping back out. Verified directly
// against real e2fsprogs output in TestEXT4ChecksumMatchesGoldenSuperblock,
// which checks this function's output against the checksum mke2fs itself
// wrote into the checked-in golden image.
func ext4Checksum(seed uint32, data []byte) uint32 {
	return ^crc32.Update(^seed, ext4ChecksumTable, data)
}

// randomEXT4UUID generates a random RFC 4122 version-4 UUID for a freshly
// formatted volume, the same way tune2fs -U random does.
func randomEXT4UUID() ([16]byte, error) {
	var uuid [16]byte
	if _, err := rand.Read(uuid[:]); err != nil {
		return uuid, err
	}
	uuid[6] = (uuid[6] & 0x0F) | 0x40 // version 4 (random)
	uuid[8] = (uuid[8] & 0x3F) | 0x80 // RFC 4122 variant
	return uuid, nil
}

// patchEXT4Superblock stamps uuid and label into an ext4SuperblockSize-byte
// superblock buffer (the primary copy or one of its sparse_super backups)
// and recomputes that copy's own checksum. It never touches
// s_checksum_seed: that field is fixed at mke2fs time and is *why* every
// other metadata checksum in the filesystem (group descriptors, inodes,
// extents, directory blocks, bitmaps) stays valid across a UUID change —
// see ext4golden/README.md's "metadata_csum_seed, verified". Only each
// superblock copy's own uuid, label and self-checksum need updating, which
// is also what tune2fs -U/-L does in practice (verified empirically while
// building this package: see the bean's Summary of Changes).
func patchEXT4Superblock(sb []byte, uuid [16]byte, label []byte) {
	copy(sb[ext4SuperblockOffUUID:ext4SuperblockOffUUID+16], uuid[:])

	labelField := sb[ext4SuperblockOffVolumeName : ext4SuperblockOffVolumeName+ext4LabelBytes]
	clear(labelField)
	copy(labelField, label)

	binary.LittleEndian.PutUint32(sb[ext4SuperblockOffChecksum:], 0)
	sum := ext4Checksum(0xFFFFFFFF, sb[:ext4SuperblockOffChecksum])
	binary.LittleEndian.PutUint32(sb[ext4SuperblockOffChecksum:], sum)
}

// requireEXT4StampableFeatures is a build-time-drift guard: FormatEXT4 only
// ever writes GoSD's own embedded golden image, which is built with both
// these features (ext4golden/README.md's "metadata_csum_seed, verified").
// If a future regeneration of the golden ever dropped either, this fails
// loudly at format time instead of silently stamping a UUID whose checksum
// implications no longer hold.
func requireEXT4StampableFeatures(sb ext4Superblock) error {
	if sb.featureIncompat&ext4IncompatMetadataCsumSeed == 0 {
		return errors.New("the embedded ext4 golden image lacks the metadata_csum_seed feature that FormatEXT4's UUID stamping assumes; internal/diskfmt needs updating before the golden image can change (see ext4golden/README.md)")
	}
	if sb.featureRoCompat&ext4RoCompatMetadataCsum == 0 {
		return errors.New("the embedded ext4 golden image lacks the metadata_csum feature that FormatEXT4's checksum stamping assumes; internal/diskfmt needs updating before the golden image can change (see ext4golden/README.md)")
	}
	return nil
}

// ext4BackupSuperblockOffsets returns the byte offsets of every backup
// superblock the golden's own geometry implies, following the sparse_super
// rule (kernel.org's filesystems/ext4/overview.html): group 1, and any group
// number that is a power of 3, 5 or 7 — every group if sparse_super is not
// set at all. A backup superblock is the first block of its group (no
// leading 1024-byte pad, unlike the primary).
//
// It refuses rather than silently mis-patch if a backup would straddle
// ext4FormatChunkBytes: see that constant's doc.
func ext4BackupSuperblockOffsets(sb ext4Superblock) ([]int64, error) {
	if sb.blocksPerGroup == 0 {
		return nil, errors.New("the embedded ext4 golden image reports 0 blocks per group")
	}
	groupBytes := uint64(sb.blocksPerGroup) * uint64(sb.blockSize)
	groupCount := (sb.blockCount + uint64(sb.blocksPerGroup) - 1) / uint64(sb.blocksPerGroup)
	sparse := sb.featureRoCompat&ext4RoCompatSparseSuper != 0

	var offsets []int64
	for g := uint64(1); g < groupCount; g++ {
		if sparse && g != 1 && !isPowerOf(g, 3) && !isPowerOf(g, 5) && !isPowerOf(g, 7) {
			continue
		}
		off := g * groupBytes
		if off%ext4FormatChunkBytes+ext4SuperblockSize > ext4FormatChunkBytes {
			return nil, fmt.Errorf("backup superblock for block group %d falls at byte offset %d, which straddles this code's %d-byte streaming chunk boundary; ext4FormatChunkBytes needs updating for a golden image with %d-byte block groups", g, off, ext4FormatChunkBytes, groupBytes)
		}
		offsets = append(offsets, int64(off))
	}
	return offsets, nil
}

// isPowerOf reports whether n is base raised to a non-negative integer
// power (1 counts, as base^0).
func isPowerOf(n, base uint64) bool {
	if n < 1 {
		return false
	}
	for n%base == 0 {
		n /= base
	}
	return n == 1
}

// FormatEXT4 formats the block device (or image file) at devicePath as a
// whole-device ext4 filesystem labelled volumeLabel, discarding any existing
// contents. It streams GoSD's checked-in golden image (see
// internal/diskfmt/ext4golden) onto the device rather than running mkfs.ext4
// — see that package's README.md for why — then stamps volumeLabel and a
// fresh random UUID into the primary superblock and its sparse_super backup
// copies.
//
// The device is opened and sized by the same openDisk helper FormatFAT32 and
// FormatExFAT use. It must be at least as large as the golden image
// (internal/diskfmt/ext4golden.RawBytes, 512 MiB); FormatEXT4 does not grow
// the filesystem to fill a larger device — that is a mount-time
// EXT4_IOC_RESIZE_FS step outside this package's scope (bean gosd-1c0x).
//
// A device FormatEXT4 has written to is not yet "established" in the sense
// GoSD's crash-ordering convention uses the word: like FAT32's
// dataexpand.EstablishedMarker, a probe that decompresses and parses fine —
// even one that reads back the label and UUID just stamped — is not proof
// the whole golden image reached the medium; a crash partway through
// streaming it can leave a superblock (which lands in the first bytes
// written) that looks perfect while later blocks are truncated. FormatEXT4
// itself declares nothing established; that write → sync → marker → sync
// commit is blockmount's territory once its own marker lands (bean
// gosd-1c0x), the same way dataexpand's marker — not a FAT probe — is what
// proves a FAT32 format finished.
func FormatEXT4(devicePath, volumeLabel string) (err error) {
	d, err := openDisk(devicePath, false)
	if err != nil {
		return fmt.Errorf("opening %s for formatting failed: %w", devicePath, err)
	}
	defer func() {
		if cerr := d.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("closing %s after formatting failed: %w", devicePath, cerr)
		}
	}()

	w, err := d.Backend.Writable()
	if err != nil {
		return fmt.Errorf("opening %s for writing failed: %w", devicePath, err)
	}
	if err := writeEXT4(w, d.Size, volumeLabel); err != nil {
		return fmt.Errorf("writing an ext4 filesystem to %s failed: %w", devicePath, err)
	}
	return nil
}

// EXT4SizeLimitReason says, in one clause, why no region smaller than
// MinEXT4Bytes can hold an ext4 filesystem FormatEXT4 or WriteEXT4 writes.
// Every refusal quotes it, mirroring FAT32SizeLimitReason.
const EXT4SizeLimitReason = "the golden ext4 image GoSD ships is a fixed 512MiB seed that is grown to the partition's real size on first boot"

// MinEXT4Bytes is the smallest region FormatEXT4 or WriteEXT4 can write: the
// embedded golden image's fixed decompressed size (internal/diskfmt/ext4golden.RawBytes).
// Callers that must validate a region's size before the region exists (e.g.
// `gosd build --data-size` sizing the data partition) compare against it so an
// impossible size is refused before any bytes are written, mirroring
// MaxFAT32Bytes.
func MinEXT4Bytes() int64 { return ext4golden.RawBytes }

// WriteEXT4 streams GoSD's embedded ext4 golden image into w starting at
// whatever offset w is already positioned at: unlike FormatEXT4, which owns
// a whole device from byte 0, WriteEXT4 has no notion of "the start of the
// device" — every WriteAt it issues is relative to w, so the caller owns the
// offset entirely (e.g. internal/image embeds an ext4 filesystem inside one
// partition of a larger .img file by shifting w's offsets itself).
//
// sizeBytes is the size of the region available to write into; it must be at
// least MinEXT4Bytes(), or WriteEXT4 refuses (see EXT4SizeLimitReason).
// WriteEXT4 always writes exactly that fixed ~512MiB golden filesystem — it
// does NOT grow it to fill a larger region, even when sizeBytes exceeds
// MinEXT4Bytes(). Growing the written filesystem to the region's real size
// is a separate runtime step (EXT4_IOC_RESIZE_FS,
// internal/blockmount.GrowEXT4), entirely outside this function's scope.
//
// As with FormatEXT4, nothing WriteEXT4 writes is "established" in GoSD's
// crash-ordering sense (see FormatEXT4's doc comment for the full argument):
// a probe that reads back fine is not proof the whole image reached the
// medium, and the write → sync → marker → sync commit record that makes it
// so is blockmount's responsibility, not this function's.
func WriteEXT4(w io.WriterAt, sizeBytes int64, volumeLabel string) error {
	return writeEXT4(w, sizeBytes, volumeLabel)
}

// writeEXT4 streams the embedded ext4 golden image into w, patching the
// primary superblock and its backups with volumeLabel and a fresh random
// UUID as they pass through. It is separated from FormatEXT4 so the on-disk
// result can be built and read back in tests without a block device, exactly
// as writeExFAT is.
func writeEXT4(w io.WriterAt, sizeBytes int64, volumeLabel string) error {
	label := []byte(volumeLabel)
	if len(label) > ext4LabelBytes {
		return fmt.Errorf("volume label %q is %d bytes; ext4 labels are at most %d bytes", volumeLabel, len(label), ext4LabelBytes)
	}
	if sizeBytes < ext4golden.RawBytes {
		return fmt.Errorf("the target is %d bytes, too small for the ext4 golden image (%d bytes / %d MiB); format a larger device, or grow it first",
			sizeBytes, ext4golden.RawBytes, ext4golden.RawBytes/(1<<20))
	}

	uuid, err := randomEXT4UUID()
	if err != nil {
		return fmt.Errorf("generating a volume UUID failed: %w", err)
	}

	zr, err := zstd.NewReader(bytes.NewReader(ext4golden.Compressed))
	if err != nil {
		return fmt.Errorf("opening the embedded ext4 golden image failed: %w", err)
	}
	defer zr.Close()

	var backupOffsets []int64
	buf := make([]byte, ext4FormatChunkBytes)
	var written int64

readLoop:
	for {
		n, rerr := io.ReadFull(zr, buf)
		if n > 0 {
			chunk := buf[:n]
			offset := written

			if offset == 0 {
				sbBytes := chunk[ext4SuperblockOffset : ext4SuperblockOffset+ext4SuperblockSize]
				sb, perr := parseEXT4Superblock(sbBytes)
				if perr != nil {
					return fmt.Errorf("the embedded ext4 golden image's own superblock failed to parse: %w", perr)
				}
				if perr := requireEXT4StampableFeatures(sb); perr != nil {
					return perr
				}
				backupOffsets, perr = ext4BackupSuperblockOffsets(sb)
				if perr != nil {
					return perr
				}
				patchEXT4Superblock(sbBytes, uuid, label)
			}
			for _, bo := range backupOffsets {
				if bo >= offset && bo < offset+int64(len(chunk)) {
					rel := bo - offset
					patchEXT4Superblock(chunk[rel:rel+ext4SuperblockSize], uuid, label)
				}
			}

			if _, werr := w.WriteAt(chunk, offset); werr != nil {
				return fmt.Errorf("writing the ext4 golden image at offset %d failed: %w", offset, werr)
			}
			written += int64(n)
		}

		switch {
		case rerr == nil:
			continue readLoop
		case errors.Is(rerr, io.EOF), errors.Is(rerr, io.ErrUnexpectedEOF):
			break readLoop
		default:
			return fmt.Errorf("decompressing the embedded ext4 golden image failed: %w", rerr)
		}
	}

	if written != ext4golden.RawBytes {
		return fmt.Errorf("the embedded ext4 golden image decompressed to %d bytes, want %d — the embedded asset may be corrupt", written, ext4golden.RawBytes)
	}
	return nil
}
