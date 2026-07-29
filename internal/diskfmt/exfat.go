package diskfmt

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf16"
)

// exFAT on-disk constants, all from the Microsoft exFAT specification.
const (
	// exFATBootSectorBytes is how much of the Main Boot Sector holds fields;
	// every one of them lies inside the first 512 bytes whatever the volume's
	// actual sector size, so this is enough to read the geometry.
	exFATBootSectorBytes = 512

	// Main Boot Sector field offsets.
	exFATOffVolumeLength      = 72 // uint64, sectors
	exFATOffFatOffset         = 80 // uint32, sectors
	exFATOffFatLength         = 84 // uint32, sectors
	exFATOffClusterHeapOffset = 88 // uint32, sectors
	exFATOffClusterCount      = 92 // uint32
	exFATOffRootCluster       = 96 // uint32
	exFATOffSerialNumber      = 100
	exFATOffRevision          = 104
	exFATOffVolumeFlags       = 106
	exFATOffSectorShift       = 108
	exFATOffClusterShift      = 109
	exFATOffNumberOfFats      = 110
	exFATOffDriveSelect       = 111
	exFATOffPercentInUse      = 112
	exFATOffBootSignature     = 510

	// Directory entry types. Each entry is 32 bytes.
	exFATDirEntryBytes  = 32
	exFATEntryEndOfDir  = 0x00
	exFATEntryBitmap    = 0x81
	exFATEntryUpcase    = 0x82
	exFATEntryVolLabel  = 0x83
	exFATMaxLabelChars  = 11
	exFATFirstCluster   = 2          // clusters 0 and 1 are the FAT's media/EOF markers
	exFATEndOfChain     = 0xFFFFFFFF // any value >= 0xFFFFFFF8 terminates a chain
	exFATMaxClusterSize = 32 << 20   // the specification's ceiling

	// exFATMaxRootClusters caps the root-directory chain walk. The label is by
	// convention the first entry of the first cluster; the cap simply stops a
	// corrupt or looping FAT from spinning forever.
	exFATMaxRootClusters = 64
)

// exFATMagic is the FileSystemName field at offset 3 of the boot sector.
var exFATMagic = []byte("EXFAT   ")

var errNotExFAT = errors.New("not an exFAT volume")

// exFATGeometry is everything about an exFAT volume's layout that is needed to
// find a byte in it.
type exFATGeometry struct {
	volumeLength      uint64 // sectors
	fatOffset         uint32 // sectors
	fatLength         uint32 // sectors
	clusterHeapOffset uint32 // sectors
	clusterCount      uint32
	rootCluster       uint32
	bytesPerSector    uint32
	sectorsPerCluster uint32
}

func (g exFATGeometry) bytesPerCluster() uint64 {
	return uint64(g.bytesPerSector) * uint64(g.sectorsPerCluster)
}

// clusterOffset is the byte offset of a cluster's first byte. Cluster
// numbering starts at 2, the first cluster of the cluster heap.
func (g exFATGeometry) clusterOffset(cluster uint32) int64 {
	sector := uint64(g.clusterHeapOffset) + uint64(cluster-exFATFirstCluster)*uint64(g.sectorsPerCluster)
	return int64(sector * uint64(g.bytesPerSector))
}

// fatEntryOffset is the byte offset of a cluster's 32-bit FAT entry.
func (g exFATGeometry) fatEntryOffset(cluster uint32) int64 {
	return int64(uint64(g.fatOffset)*uint64(g.bytesPerSector) + uint64(cluster)*4)
}

// parseExFATBootSector reads a Main Boot Sector's geometry, rejecting anything
// that does not describe a volume this package can navigate.
func parseExFATBootSector(sector []byte) (exFATGeometry, error) {
	if len(sector) < exFATBootSectorBytes {
		return exFATGeometry{}, errNotExFAT
	}
	if !isExFAT(sector) {
		return exFATGeometry{}, errNotExFAT
	}
	if binary.LittleEndian.Uint16(sector[exFATOffBootSignature:]) != 0xAA55 {
		return exFATGeometry{}, fmt.Errorf("%w: boot signature missing", errNotExFAT)
	}

	sectorShift := sector[exFATOffSectorShift]
	clusterShift := sector[exFATOffClusterShift]
	if sectorShift < 9 || sectorShift > 12 {
		return exFATGeometry{}, fmt.Errorf("%w: %d-byte sectors are out of range", errNotExFAT, 1<<sectorShift)
	}
	if uint32(sectorShift)+uint32(clusterShift) > 25 {
		return exFATGeometry{}, fmt.Errorf("%w: cluster size exceeds the 32 MiB maximum", errNotExFAT)
	}

	g := exFATGeometry{
		volumeLength:      binary.LittleEndian.Uint64(sector[exFATOffVolumeLength:]),
		fatOffset:         binary.LittleEndian.Uint32(sector[exFATOffFatOffset:]),
		fatLength:         binary.LittleEndian.Uint32(sector[exFATOffFatLength:]),
		clusterHeapOffset: binary.LittleEndian.Uint32(sector[exFATOffClusterHeapOffset:]),
		clusterCount:      binary.LittleEndian.Uint32(sector[exFATOffClusterCount:]),
		rootCluster:       binary.LittleEndian.Uint32(sector[exFATOffRootCluster:]),
		bytesPerSector:    1 << sectorShift,
		sectorsPerCluster: 1 << clusterShift,
	}
	if g.fatLength == 0 || g.clusterCount == 0 {
		return exFATGeometry{}, fmt.Errorf("%w: empty FAT or cluster heap", errNotExFAT)
	}
	if g.rootCluster < exFATFirstCluster || g.rootCluster > g.clusterCount+1 {
		return exFATGeometry{}, fmt.Errorf("%w: root directory cluster %d is outside the cluster heap", errNotExFAT, g.rootCluster)
	}
	return g, nil
}

// readExFATLabel returns the volume label of the exFAT volume on devicePath.
// An exFAT volume with no label entry has an empty label, which is not an
// error.
func readExFATLabel(devicePath string) (string, error) {
	f, err := os.Open(devicePath)
	if err != nil {
		return "", fmt.Errorf("opening %s to read its exFAT label failed: %w", devicePath, err)
	}
	defer func() { _ = f.Close() }()
	return exFATLabel(f)
}

// exFATLabel walks an exFAT volume's root directory for its 0x83 volume-label
// entry. The root directory is a normal FAT-chained cluster chain, so this
// needs the boot sector's geometry to locate both the chain and its links.
func exFATLabel(r io.ReaderAt) (string, error) {
	boot := make([]byte, exFATBootSectorBytes)
	if _, err := r.ReadAt(boot, 0); err != nil {
		return "", fmt.Errorf("reading the exFAT boot sector failed: %w", err)
	}
	g, err := parseExFATBootSector(boot)
	if err != nil {
		return "", err
	}

	clusterBytes := g.bytesPerCluster()
	if clusterBytes > exFATMaxClusterSize {
		return "", fmt.Errorf("%w: cluster size %d exceeds the 32 MiB maximum", errNotExFAT, clusterBytes)
	}
	cluster := g.rootCluster
	buf := make([]byte, clusterBytes)

	for walked := 0; walked < exFATMaxRootClusters; walked++ {
		if cluster < exFATFirstCluster || cluster > g.clusterCount+1 {
			return "", fmt.Errorf("%w: root directory chain leaves the cluster heap", errNotExFAT)
		}
		if _, err := r.ReadAt(buf, g.clusterOffset(cluster)); err != nil {
			return "", fmt.Errorf("reading the exFAT root directory failed: %w", err)
		}
		for at := 0; at+exFATDirEntryBytes <= len(buf); at += exFATDirEntryBytes {
			entry := buf[at : at+exFATDirEntryBytes]
			switch entry[0] {
			case exFATEntryEndOfDir:
				return "", nil
			case exFATEntryVolLabel:
				return decodeExFATLabel(entry), nil
			}
		}

		next, err := readFATEntry(r, g, cluster)
		if err != nil {
			return "", err
		}
		if next >= 0xFFFFFFF8 {
			return "", nil
		}
		cluster = next
	}
	return "", fmt.Errorf("%w: root directory chain is longer than %d clusters", errNotExFAT, exFATMaxRootClusters)
}

func readFATEntry(r io.ReaderAt, g exFATGeometry, cluster uint32) (uint32, error) {
	var raw [4]byte
	if _, err := r.ReadAt(raw[:], g.fatEntryOffset(cluster)); err != nil {
		return 0, fmt.Errorf("reading the exFAT FAT failed: %w", err)
	}
	return binary.LittleEndian.Uint32(raw[:]), nil
}

// decodeExFATLabel reads a 0x83 volume-label entry: a character count followed
// by that many UTF-16LE code units.
func decodeExFATLabel(entry []byte) string {
	count := int(entry[1])
	if count > exFATMaxLabelChars {
		count = exFATMaxLabelChars
	}
	units := make([]uint16, count)
	for i := range units {
		units[i] = binary.LittleEndian.Uint16(entry[2+2*i:])
	}
	return strings.TrimRight(string(utf16.Decode(units)), " \x00")
}
