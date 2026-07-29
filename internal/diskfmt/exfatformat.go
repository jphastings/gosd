package diskfmt

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"math/bits"
	"slices"
	"unicode"
	"unicode/utf16"

	"github.com/diskfs/go-diskfs"
)

const (
	// exFATFormatSectorShift fixes the sector size FormatExFAT writes at 512
	// bytes, matching the FAT32 path's diskfs.SectorSize512. Every board GoSD
	// targets presents its media with 512-byte logical sectors.
	exFATFormatSectorShift = 9
	exFATFormatSectorSize  = 1 << exFATFormatSectorShift

	// exFATBootRegionSectors is the Main Boot Region: the boot sector, eight
	// extended boot sectors, the OEM parameters and reserved sectors, and the
	// checksum sector. The Backup Boot Region is a copy of it, immediately
	// after.
	exFATBootRegionSectors  = 12
	exFATBootChecksumSector = 11

	// exFATFormatFatOffset is the sector the FAT starts at. The specification's
	// minimum is 24 — just past both boot regions — and rounding up to 128
	// leaves the customary gap without wasting meaningful space.
	exFATFormatFatOffset = 128

	// exFATMaxClusterCount is the specification's ceiling of 2^32 - 11.
	exFATMaxClusterCount = 0xFFFFFFF5

	// exFATMinVolumeBytes is the specification's smallest volume, and comfortably
	// larger than the metadata a format has to fit.
	exFATMinVolumeBytes = 1 << 20

	// exFATSpanChunkBytes bounds how much is held in memory while writing a
	// region that can be tens of megabytes on a large disk.
	exFATSpanChunkBytes = 1 << 20
)

// exFATLayout is a complete plan for a volume: where every region goes and how
// much of the cluster heap the format itself occupies.
type exFATLayout struct {
	geometry       exFATGeometry
	clusterShift   uint8
	bitmapCluster  uint32
	bitmapClusters uint32
	bitmapBytes    uint64
	upcaseCluster  uint32
	upcaseClusters uint32
	upcaseBytes    uint64
	usedClusters   uint32
	serial         uint32
}

// FormatExFAT formats the block device (or image file) at devicePath as a
// single whole-device exFAT filesystem labelled volumeLabel, discarding any
// existing contents.
//
// It writes no partition table, exactly as FormatFAT32 does, so the whole-device
// node stays directly shareable over USB mass storage and no privileged
// partition-table reread is needed. go-diskfs has no exFAT support, so only its
// device-size detection is used here; the filesystem itself is written from the
// Microsoft exFAT specification.
func FormatExFAT(devicePath, volumeLabel string) (err error) {
	d, err := diskfs.Open(devicePath,
		diskfs.WithOpenMode(diskfs.ReadWrite),
		diskfs.WithSectorSize(diskfs.SectorSize512),
	)
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
	if err := writeExFAT(w, d.Size, volumeLabel); err != nil {
		return fmt.Errorf("writing an exFAT filesystem to %s failed: %w", devicePath, err)
	}
	return nil
}

// writeExFAT lays a whole-device exFAT filesystem of sizeBytes into w. It is
// separated from FormatExFAT so the on-disk result can be built and read back
// in tests without a block device.
func writeExFAT(w io.WriterAt, sizeBytes int64, volumeLabel string) error {
	label := utf16.Encode([]rune(volumeLabel))
	if len(label) > exFATMaxLabelChars {
		return fmt.Errorf("volume label %q is %d characters; exFAT labels are at most %d", volumeLabel, len(label), exFATMaxLabelChars)
	}

	upcase := exFATUpcaseTable()
	l, err := newExFATLayout(sizeBytes, uint64(len(upcase)))
	if err != nil {
		return err
	}
	g := l.geometry

	// The Backup Boot Region is a byte-for-byte copy, which is what lets a
	// reader recover from a damaged main region.
	boot := l.bootRegion()
	for _, at := range []int64{0, exFATBootRegionSectors * exFATFormatSectorSize} {
		if _, err := w.WriteAt(boot, at); err != nil {
			return fmt.Errorf("writing the boot region at offset %d failed: %w", at, err)
		}
	}

	// Nothing lives between the backup boot region and the FAT, but a previous
	// filesystem's remains there would mislead anything that goes looking.
	gapStart := int64(2 * exFATBootRegionSectors * exFATFormatSectorSize)
	fatStart := int64(uint64(g.fatOffset) * exFATFormatSectorSize)
	if err := writeSpan(w, gapStart, uint64(fatStart-gapStart), nil); err != nil {
		return fmt.Errorf("clearing the reserved region failed: %w", err)
	}

	if err := writeSpan(w, fatStart, uint64(g.fatLength)*exFATFormatSectorSize, l.fillFAT); err != nil {
		return fmt.Errorf("writing the FAT failed: %w", err)
	}
	if err := writeSpan(w, g.clusterOffset(l.bitmapCluster), uint64(l.bitmapClusters)*g.bytesPerCluster(), l.fillBitmap); err != nil {
		return fmt.Errorf("writing the allocation bitmap failed: %w", err)
	}
	if err := writeSpan(w, g.clusterOffset(l.upcaseCluster), uint64(l.upcaseClusters)*g.bytesPerCluster(), func(chunk []byte) error {
		if len(chunk) < len(upcase) {
			return fmt.Errorf("the up-case table is %d bytes, more than the %d reserved for it", len(upcase), len(chunk))
		}
		copy(chunk, upcase)
		return nil
	}); err != nil {
		return fmt.Errorf("writing the up-case table failed: %w", err)
	}

	root := l.rootDirectory(label, exFATRollingChecksum(upcase))
	if _, err := w.WriteAt(root, g.clusterOffset(g.rootCluster)); err != nil {
		return fmt.Errorf("writing the root directory failed: %w", err)
	}
	return nil
}

// newExFATLayout plans a volume of sizeBytes whose up-case table needs
// upcaseBytes.
func newExFATLayout(sizeBytes int64, upcaseBytes uint64) (exFATLayout, error) {
	if sizeBytes < exFATMinVolumeBytes {
		return exFATLayout{}, fmt.Errorf("exFAT needs a volume of at least %d bytes; this one is %d", exFATMinVolumeBytes, sizeBytes)
	}
	volumeLength := uint64(sizeBytes) / exFATFormatSectorSize
	clusterShift := exFATClusterShift(sizeBytes)
	sectorsPerCluster := uint64(1) << clusterShift
	clusterBytes := sectorsPerCluster * exFATFormatSectorSize

	// FatLength is sized against an upper bound on the cluster count — what the
	// volume would hold if the FAT itself took no room. Sectors spent on the FAT
	// only ever reduce the cluster count, so this bound is always sufficient,
	// and it breaks the circular dependency between the two fields.
	maxClusters := (volumeLength - exFATFormatFatOffset) >> clusterShift
	fatLength := divCeil(maxClusters+2, exFATFormatSectorSize/4)
	heapOffset := alignUp(exFATFormatFatOffset+fatLength, sectorsPerCluster)
	if heapOffset > math.MaxUint32 || fatLength > math.MaxUint32 {
		return exFATLayout{}, fmt.Errorf("a %d-byte volume needs a FAT larger than exFAT can address", sizeBytes)
	}
	if heapOffset+sectorsPerCluster > volumeLength {
		return exFATLayout{}, fmt.Errorf("a %d-byte volume leaves no room for exFAT's cluster heap", sizeBytes)
	}
	clusterCount := min((volumeLength-heapOffset)>>clusterShift, exFATMaxClusterCount)

	bitmapBytes := divCeil(clusterCount, 8)
	bitmapClusters := divCeil(bitmapBytes, clusterBytes)
	upcaseClusters := divCeil(upcaseBytes, clusterBytes)
	usedClusters := bitmapClusters + upcaseClusters + 1 // + the root directory
	if usedClusters > clusterCount {
		return exFATLayout{}, fmt.Errorf("a %d-byte volume is too small for exFAT's own metadata", sizeBytes)
	}

	var serial [4]byte
	if _, err := rand.Read(serial[:]); err != nil {
		return exFATLayout{}, fmt.Errorf("generating a volume serial number failed: %w", err)
	}

	return exFATLayout{
		geometry: exFATGeometry{
			volumeLength:      volumeLength,
			fatOffset:         exFATFormatFatOffset,
			fatLength:         uint32(fatLength),
			clusterHeapOffset: uint32(heapOffset),
			clusterCount:      uint32(clusterCount),
			rootCluster:       uint32(exFATFirstCluster + bitmapClusters + upcaseClusters),
			bytesPerSector:    exFATFormatSectorSize,
			sectorsPerCluster: uint32(sectorsPerCluster),
		},
		clusterShift:   clusterShift,
		bitmapCluster:  exFATFirstCluster,
		bitmapClusters: uint32(bitmapClusters),
		bitmapBytes:    bitmapBytes,
		upcaseCluster:  uint32(exFATFirstCluster + bitmapClusters),
		upcaseClusters: uint32(upcaseClusters),
		upcaseBytes:    upcaseBytes,
		usedClusters:   uint32(usedClusters),
		serial:         binary.LittleEndian.Uint32(serial[:]),
	}, nil
}

// exFATClusterShift is Microsoft's recommended cluster size for a volume,
// expressed as a shift over 512-byte sectors: 4 KiB up to 256 MiB, 32 KiB up to
// 32 GiB, 128 KiB beyond.
func exFATClusterShift(sizeBytes int64) uint8 {
	switch {
	case sizeBytes <= 256<<20:
		return 3
	case sizeBytes <= 32<<30:
		return 6
	default:
		return 8
	}
}

// bootRegion builds the 12-sector boot region, including the checksum sector
// that every reader validates the other eleven against.
func (l exFATLayout) bootRegion() []byte {
	const sector = exFATFormatSectorSize
	g := l.geometry
	region := make([]byte, exFATBootRegionSectors*sector)

	boot := region[:sector]
	copy(boot[0:3], []byte{0xEB, 0x76, 0x90}) // JumpBoot
	copy(boot[3:11], exFATMagic)
	binary.LittleEndian.PutUint64(boot[exFATOffVolumeLength:], g.volumeLength)
	binary.LittleEndian.PutUint32(boot[exFATOffFatOffset:], g.fatOffset)
	binary.LittleEndian.PutUint32(boot[exFATOffFatLength:], g.fatLength)
	binary.LittleEndian.PutUint32(boot[exFATOffClusterHeapOffset:], g.clusterHeapOffset)
	binary.LittleEndian.PutUint32(boot[exFATOffClusterCount:], g.clusterCount)
	binary.LittleEndian.PutUint32(boot[exFATOffRootCluster:], g.rootCluster)
	binary.LittleEndian.PutUint32(boot[exFATOffSerialNumber:], l.serial)
	binary.LittleEndian.PutUint16(boot[exFATOffRevision:], 0x0100) // version 1.00
	boot[exFATOffSectorShift] = exFATFormatSectorShift
	boot[exFATOffClusterShift] = l.clusterShift
	boot[exFATOffNumberOfFats] = 1
	boot[exFATOffDriveSelect] = 0x80
	boot[exFATOffPercentInUse] = l.percentInUse()
	binary.LittleEndian.PutUint16(boot[exFATOffBootSignature:], 0xAA55)

	// The eight extended boot sectors carry no boot code, only their signature.
	for s := 1; s <= 8; s++ {
		binary.LittleEndian.PutUint32(region[(s+1)*sector-4:], 0xAA550000)
	}
	// Sector 9 (OEM parameters) and sector 10 (reserved) stay zero.

	// VolumeFlags and PercentInUse are excluded from the checksum, so a driver
	// may update them in place without recomputing it.
	sum := exFATRollingChecksum(region[:exFATBootChecksumSector*sector],
		exFATOffVolumeFlags, exFATOffVolumeFlags+1, exFATOffPercentInUse)
	checksum := region[exFATBootChecksumSector*sector:]
	for at := 0; at+4 <= len(checksum); at += 4 {
		binary.LittleEndian.PutUint32(checksum[at:], sum)
	}
	return region
}

// rootDirectory builds the root directory cluster: the volume label, and the
// two entries that tell a reader where the allocation bitmap and up-case table
// live. Everything after them is zero, which marks the end of the directory.
func (l exFATLayout) rootDirectory(label []uint16, upcaseChecksum uint32) []byte {
	dir := make([]byte, l.geometry.bytesPerCluster())

	volume := dir[0:exFATDirEntryBytes]
	volume[0] = exFATEntryVolLabel
	volume[1] = byte(len(label))
	for i, unit := range label {
		binary.LittleEndian.PutUint16(volume[2+2*i:], unit)
	}

	bitmap := dir[exFATDirEntryBytes : 2*exFATDirEntryBytes]
	bitmap[0] = exFATEntryBitmap
	binary.LittleEndian.PutUint32(bitmap[20:], l.bitmapCluster)
	binary.LittleEndian.PutUint64(bitmap[24:], l.bitmapBytes)

	upcase := dir[2*exFATDirEntryBytes : 3*exFATDirEntryBytes]
	upcase[0] = exFATEntryUpcase
	binary.LittleEndian.PutUint32(upcase[4:], upcaseChecksum)
	binary.LittleEndian.PutUint32(upcase[20:], l.upcaseCluster)
	binary.LittleEndian.PutUint64(upcase[24:], l.upcaseBytes)

	return dir
}

// fillFAT writes the FAT's fixed head: the two reserved entries, then a chain
// per metadata region. Every other cluster is free, which the zeroed remainder
// of the FAT and the allocation bitmap agree on.
func (l exFATLayout) fillFAT(chunk []byte) error {
	binary.LittleEndian.PutUint32(chunk[0:], 0xFFFFFFF8) // media descriptor
	binary.LittleEndian.PutUint32(chunk[4:], exFATEndOfChain)

	chain := func(first, count uint32) error {
		for i := uint32(0); i < count; i++ {
			at := (uint64(first) + uint64(i)) * 4
			if at+4 > uint64(len(chunk)) {
				return fmt.Errorf("the metadata cluster chains do not fit in the FAT's first %d bytes", len(chunk))
			}
			next := uint32(exFATEndOfChain)
			if i+1 < count {
				next = first + i + 1
			}
			binary.LittleEndian.PutUint32(chunk[at:], next)
		}
		return nil
	}
	if err := chain(l.bitmapCluster, l.bitmapClusters); err != nil {
		return err
	}
	if err := chain(l.upcaseCluster, l.upcaseClusters); err != nil {
		return err
	}
	return chain(l.geometry.rootCluster, 1)
}

// fillBitmap marks the clusters the format itself occupies as allocated. Bit i
// of the bitmap is cluster i+2, the first cluster of the heap.
func (l exFATLayout) fillBitmap(chunk []byte) error {
	if uint64(divCeil(uint64(l.usedClusters), 8)) > uint64(len(chunk)) {
		return fmt.Errorf("the %d in-use clusters do not fit in the bitmap's first %d bytes", l.usedClusters, len(chunk))
	}
	for i := uint32(0); i < l.usedClusters; i++ {
		chunk[i/8] |= 1 << (i % 8)
	}
	return nil
}

func (l exFATLayout) percentInUse() uint8 {
	return uint8(uint64(l.usedClusters) * 100 / uint64(l.geometry.clusterCount))
}

// exFATRollingChecksum is the specification's 32-bit rolling checksum, used
// both for the boot region (which excludes the offsets a driver may rewrite)
// and for the up-case table (which excludes none).
func exFATRollingChecksum(data []byte, skipped ...int) uint32 {
	var sum uint32
	for i, b := range data {
		if slices.Contains(skipped, i) {
			continue
		}
		sum = bits.RotateLeft32(sum, -1) + uint32(b)
	}
	return sum
}

// exFATUpcaseTable builds the volume's up-case table in the specification's
// run-length compressed form, where a 0xFFFF marker followed by a count means
// that many characters in a row map to themselves.
//
// The table is generated from Go's own Unicode data rather than embedding
// Microsoft's recommended byte sequence: every reader loads the table from the
// volume and validates it against the checksum stored beside it, so nothing
// requires those exact bytes. What is required is coverage — Linux's exfat
// driver rejects a table that stops short of the whole Basic Multilingual
// Plane — which is why this walks all 65,536 code points rather than just the
// ASCII range the specification makes mandatory.
func exFATUpcaseTable() []byte {
	out := make([]byte, 0, 8<<10)
	identity := 0
	flush := func() {
		for identity > 0 {
			run := min(identity, 0xFFFF)
			out = binary.LittleEndian.AppendUint16(out, 0xFFFF)
			out = binary.LittleEndian.AppendUint16(out, uint16(run))
			identity -= run
		}
	}
	for c := range 0x10000 {
		up := exFATUpcase(rune(c))
		if up == uint16(c) {
			identity++
			continue
		}
		flush()
		out = binary.LittleEndian.AppendUint16(out, up)
	}
	flush()
	return out
}

// exFATUpcase is a character's simple uppercase mapping, restricted to
// mappings that stay inside the Basic Multilingual Plane — the only ones a
// 16-bit table can express.
func exFATUpcase(r rune) uint16 {
	if r >= 0xD800 && r <= 0xDFFF {
		return uint16(r) // surrogate halves are not characters and have no case
	}
	up := unicode.ToUpper(r)
	if up == r || up < 0 || up > 0xFFFF {
		return uint16(r)
	}
	return uint16(up)
}

// writeSpan writes size bytes at off. fill, if given, is handed the first chunk
// to populate; everything after it is zeroed, which is what stops a previous
// filesystem's data being mistaken for this one's.
func writeSpan(w io.WriterAt, off int64, size uint64, fill func(chunk []byte) error) error {
	if size == 0 {
		return nil
	}
	chunk := min(uint64(exFATSpanChunkBytes), size)
	buf := make([]byte, chunk)
	if fill != nil {
		if err := fill(buf); err != nil {
			return err
		}
	}
	for written := uint64(0); written < size; {
		n := min(chunk, size-written)
		if _, err := w.WriteAt(buf[:n], off+int64(written)); err != nil {
			return err
		}
		written += n
		if written == chunk {
			clear(buf) // only the first chunk carries content
		}
	}
	return nil
}

func divCeil(n, by uint64) uint64 { return (n + by - 1) / by }

func alignUp(n, to uint64) uint64 { return divCeil(n, to) * to }
