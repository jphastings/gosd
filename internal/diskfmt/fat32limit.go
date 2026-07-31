package diskfmt

import (
	"fmt"
	"math"
)

// go-diskfs lays a FAT32 volume out by computing how many sectors each of the
// two file allocation tables needs, then narrowing that count to a uint16
// (v1.9.3's fat32.Create). FATSz32 is a 32-bit field on disk, so the narrowing
// is an implementation slip rather than a FAT32 limit — but past 65535 sectors
// per FAT it silently truncates, laying out FATs far too small to address the
// volume's clusters, and writes the corrupt result without complaint. Higher
// still, around 512 GiB, the same expression's uint32 numerator wraps and
// Create panics on a zero-length FAT instead. go-diskfs's own Fat32MaxSize
// (2 TiB) catches neither.
//
// The arithmetic is mirrored here so oversized media is refused before a byte
// is written. It works out at just over 256 GiB — which is why
// cmd/gosd-init/internal/dataexpand caps a grown data partition at a round
// 256 GiB. Both this guard and that cap can go once the go-diskfs pin carries
// the upstream fix (bean gosd-8kdm).
const (
	// fat32ReservedSectors is the reserved area go-diskfs fixes every FAT32
	// volume's layout on: boot sector, FSInfo and their backups, rounded up.
	fat32ReservedSectors = 32

	// fat32MaxSectorsPerFAT is the largest per-FAT sector count go-diskfs can
	// express before that uint16 wraps.
	fat32MaxSectorsPerFAT = math.MaxUint16
)

// maxFAT32Bytes is the largest volume FormatFAT32 will create: the size whose
// per-FAT sector count still fits go-diskfs's uint16, found by inverting
// fat32SectorsPerFAT at the 32 KiB cluster size every volume near the limit
// gets (the top class of fat32SectorsPerCluster, which math.MaxInt64 selects).
var maxFAT32Bytes = (fat32MaxSectorsPerFAT*fat32PerFATDenominator(math.MaxInt64)/4 + fat32ReservedSectors) * sectorSizeBytes

// FAT32SizeLimitReason says, in one clause, why no FAT32 volume larger than
// MaxFAT32Bytes may be written. Every refusal quotes it — the runtime one below
// and `gosd build --data-size`'s, which lands long before a device exists — so
// an app author who meets the limit twice reads one story rather than two.
const FAT32SizeLimitReason = "GoSD's FAT32 formatter counts the sectors in each file allocation table in 16 bits, so a larger volume would be laid out with FATs far too small for it and silently corrupted"

// MaxFAT32Bytes is the largest FAT32 volume GoSD will create. Callers that fix
// a volume's size before the volume exists — `gosd build --data-size` sizing
// GOSD-DATA — compare against it so an impossible size is refused at the flag
// rather than written into an image.
func MaxFAT32Bytes() int64 { return maxFAT32Bytes }

// GibibytesString renders a byte count the way storage sizes are talked about,
// with enough precision to tell a volume just over the FAT32 limit from one
// just under it.
func GibibytesString(bytes int64) string {
	return fmt.Sprintf("%.2f GiB", float64(bytes)/(1<<30))
}

// checkFAT32Size reports whether a FAT32 volume spanning sizeBytes can be laid
// out correctly, naming the device so the refusal is specific.
func checkFAT32Size(devicePath string, sizeBytes int64) error {
	if fat32SectorsPerFAT(sizeBytes) <= fat32MaxSectorsPerFAT {
		return nil
	}
	return fmt.Errorf("%s is %s, and GoSD cannot create a FAT32 volume larger than %s: %s; format this device as exFAT instead (disk.Options{Filesystem: disk.ExFAT}), which has no such limit and suits media this large",
		devicePath, GibibytesString(sizeBytes), GibibytesString(maxFAT32Bytes), FAT32SizeLimitReason)
}

// fat32SectorsPerFAT is the number of sectors each of the two FATs needs for a
// volume of sizeBytes: go-diskfs's own closed form of the dosfstools mkfs.fat
// search, in full width rather than truncated to a uint16.
func fat32SectorsPerFAT(sizeBytes int64) int64 {
	totalSectors := sizeBytes / sectorSizeBytes
	if totalSectors <= fat32ReservedSectors {
		return 0
	}
	denominator := fat32PerFATDenominator(sizeBytes)
	return (4*(totalSectors-fat32ReservedSectors) + denominator - 1) / denominator
}

// fat32PerFATDenominator is the divisor in that closed form: the bytes one
// cluster costs, plus the 8 bytes of FAT entry the two tables spend on it.
func fat32PerFATDenominator(sizeBytes int64) int64 {
	return sectorSizeBytes*fat32SectorsPerCluster(sizeBytes) + 8
}

// fat32SectorsPerCluster mirrors the cluster size go-diskfs picks for a volume
// of sizeBytes — the same table dosfstools' mkfs.fat and Microsoft's format
// use.
func fat32SectorsPerCluster(sizeBytes int64) int64 {
	const (
		kib = 1024
		mib = 1024 * kib
		gib = 1024 * mib
	)
	var clusterBytes int64
	switch {
	case sizeBytes <= 260*mib:
		clusterBytes = 512
	case sizeBytes <= 8*gib:
		clusterBytes = 4 * kib
	case sizeBytes <= 16*gib:
		clusterBytes = 8 * kib
	case sizeBytes <= 32*gib:
		clusterBytes = 16 * kib
	default:
		clusterBytes = 32 * kib
	}
	if clusterBytes < sectorSizeBytes {
		return 1
	}
	return clusterBytes / sectorSizeBytes
}
