package diskfmt

import (
	"encoding/binary"
	"os"
	"testing"
)

// fat32BPB is the handful of BIOS Parameter Block fields that decide whether a
// FAT32 volume can address itself, read straight off a formatted device.
type fat32BPB struct {
	bytesPerSector    int64
	sectorsPerCluster int64
	reservedSectors   int64
	fatCount          int64
	totalSectors      int64
	sectorsPerFAT     int64
}

// clusters is how many data clusters the volume advertises, and entries how
// many of them one FAT can index. A volume is only usable when the second is at
// least the first plus two, since FAT entries 0 and 1 are reserved markers.
func (b fat32BPB) clusters() int64 {
	return (b.totalSectors - b.reservedSectors - b.fatCount*b.sectorsPerFAT) / b.sectorsPerCluster
}

func (b fat32BPB) entries() int64 { return b.sectorsPerFAT * b.bytesPerSector / 4 }

func readFAT32BPB(t *testing.T, devicePath string) fat32BPB {
	t.Helper()
	f, err := os.Open(devicePath)
	if err != nil {
		t.Fatalf("reopening formatted device: %v", err)
	}
	defer func() { _ = f.Close() }()

	sector := make([]byte, sectorSizeBytes)
	if _, err := f.ReadAt(sector, 0); err != nil {
		t.Fatalf("reading boot sector back: %v", err)
	}
	return fat32BPB{
		bytesPerSector:    int64(binary.LittleEndian.Uint16(sector[11:13])),
		sectorsPerCluster: int64(sector[13]),
		reservedSectors:   int64(binary.LittleEndian.Uint16(sector[14:16])),
		fatCount:          int64(sector[16]),
		totalSectors:      int64(binary.LittleEndian.Uint32(sector[32:36])),
		sectorsPerFAT:     int64(binary.LittleEndian.Uint32(sector[36:40])),
	}
}

// TestFormatFAT32WritesAFATThatIndexesEveryClusterItAdvertises is the point of
// the mitigation: go-diskfs under-sized the FAT at ~0.8% of volume sizes, and
// the round ones real cards and `--data-size` land on are over-represented
// among them. macOS fsck_msdos and Windows chkdsk both report such a volume as
// damaged; here the check is the same arithmetic they apply.
func TestFormatFAT32WritesAFATThatIndexesEveryClusterItAdvertises(t *testing.T) {
	const gib = 1 << 30
	for _, tc := range []struct {
		name string
		size int64
	}{
		{"256 MiB, the GOSD-BOOT partition size", 256 << 20},
		{"1 GiB", 1 * gib},
		{"8 GiB", 8 * gib},
		{"16 GiB", 16 * gib},
		{"32 GiB", 32 * gib},
		{"64 GiB", 64 * gib},
		{"256 GiB, the cap dataexpand grows a partition to", 256 * gib},
		{"the largest volume GoSD will create", maxFAT32Bytes},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := backingFile(t, tc.size) // sparse: only the FATs cost real bytes
			if err := FormatFAT32(path, "GOSD-DATA"); err != nil {
				t.Fatalf("FormatFAT32: %v", err)
			}

			bpb := readFAT32BPB(t, path)
			if got, want := bpb.entries(), bpb.clusters()+2; got < want {
				t.Errorf("the FAT holds %d entries for a volume of %d clusters; %d entries are needed to address them all",
					got, bpb.clusters(), want)
			}
			if bpb.sectorsPerFAT > fat32MaxSectorsPerFAT {
				t.Errorf("sectors per FAT = %d, past the %d go-diskfs can record", bpb.sectorsPerFAT, int64(fat32MaxSectorsPerFAT))
			}

			// Whatever it costs to get there, the volume still has to span
			// essentially all of the device.
			lost := tc.size/sectorSizeBytes - bpb.totalSectors
			if lost < 0 || lost > 2*bpb.sectorsPerCluster {
				t.Errorf("the volume is %d sectors short of the device, more than the 2 clusters (%d sectors) the mitigation may cost",
					lost, 2*bpb.sectorsPerCluster)
			}
		})
	}
}

// fat32FATIndexesEveryCluster applies that same check to the layout go-diskfs
// would produce for a volume of sizeBytes, without writing one.
func fat32FATIndexesEveryCluster(sizeBytes int64) bool {
	sectorsPerFAT := fat32SectorsPerFAT(sizeBytes)
	dataSectors := sizeBytes/sectorSizeBytes - fat32ReservedSectors - 2*sectorsPerFAT
	if dataSectors <= 0 {
		return false
	}
	clusters := dataSectors / fat32SectorsPerCluster(sizeBytes)
	return sectorsPerFAT*(sectorSizeBytes/4) >= clusters+2
}

// TestLargestSelfConsistentFAT32BytesTrimsExactlyTheDefectiveTopOfEachBand
// pins the boundary the whole mitigation rests on: go-diskfs's sectors-per-FAT
// count rises in bands of 128*sectorsPerCluster+2 sectors, and exactly the top
// sectorsPerCluster+1 sectors of each band are laid out with a FAT one entry
// too small. Sizes below that are untouched; sizes within it fall back to the
// last good sector.
func TestLargestSelfConsistentFAT32BytesTrimsExactlyTheDefectiveTopOfEachBand(t *testing.T) {
	for _, near := range []int64{
		200 << 20,  // 512-byte clusters
		4 << 30,    // 4 KiB
		12 << 30,   // 8 KiB
		24 << 30,   // 16 KiB
		64 << 30,   // 32 KiB
		1000 << 30, // past the FAT32 ceiling: still answerable, still trimmed
	} {
		sectorsPerCluster := fat32SectorsPerCluster(near)
		bandTop := fat32SectorsPerFAT(near) * (fat32PerFATDenominator(near) / 4)
		lastGood := bandTop - (sectorsPerCluster + 1)

		sizeOf := func(nonReservedSectors int64) int64 {
			return (nonReservedSectors + fat32ReservedSectors) * sectorSizeBytes
		}

		for n := lastGood - 1; n <= bandTop; n++ {
			size := sizeOf(n)
			want := sizeOf(min(n, lastGood))
			wantSelfConsistent := n <= lastGood

			if got := LargestSelfConsistentFAT32Bytes(size); got != want {
				t.Fatalf("LargestSelfConsistentFAT32Bytes(%d) = %d, want %d (%d-sector clusters, band top %d)",
					size, got, want, sectorsPerCluster, bandTop)
			}
			if got := fat32FATIndexesEveryCluster(size); got != wantSelfConsistent {
				t.Fatalf("a volume of %d non-reserved sectors (%d-sector clusters) is self-consistent = %v, want %v",
					n, sectorsPerCluster, got, wantSelfConsistent)
			}
			if size-want > 2*sectorsPerCluster*sectorSizeBytes {
				t.Fatalf("trimming %d bytes cost %d bytes, more than 2 clusters", size, size-want)
			}
		}
	}
}

// TestLargestSelfConsistentFAT32BytesLeavesTheBigRoundSizesFormattable sweeps
// the sizes an app author actually asks for, at every cluster size, and the
// FAT32 ceiling itself — which was one of the defective sizes, so the guard's
// own boundary could not be formatted correctly before this.
func TestLargestSelfConsistentFAT32BytesLeavesTheBigRoundSizesFormattable(t *testing.T) {
	sizes := []int64{maxFAT32Bytes}
	for gib := int64(1); gib <= 256; gib++ {
		sizes = append(sizes, gib<<30)
	}
	for _, size := range sizes {
		got := LargestSelfConsistentFAT32Bytes(size)
		if got > size {
			t.Fatalf("LargestSelfConsistentFAT32Bytes(%d) = %d, larger than the device", size, got)
		}
		if !fat32FATIndexesEveryCluster(got) {
			t.Fatalf("a volume of %d bytes trimmed to %d, which still cannot address its own clusters", size, got)
		}
		if lost := size - got; lost > 2*fat32SectorsPerCluster(got)*sectorSizeBytes {
			t.Fatalf("trimming %d bytes cost %d bytes, more than 2 clusters", size, lost)
		}
	}
}

// TestFormatFAT32StillRefusesPastTheCeilingAfterTrimming: trimming may never
// turn an oversized device into an accepted one — a device past MaxFAT32Bytes
// is still far past it two clusters down.
func TestFormatFAT32StillRefusesPastTheCeilingAfterTrimming(t *testing.T) {
	for _, size := range []int64{maxFAT32Bytes + sectorSizeBytes, 512_000_000_000} {
		if err := checkFAT32Size("/dev/sda", size); err == nil {
			t.Errorf("a %s device was accepted; want a refusal", GibibytesString(size))
		}
		if trimmed := LargestSelfConsistentFAT32Bytes(size); fat32SectorsPerFAT(trimmed) <= fat32MaxSectorsPerFAT {
			t.Errorf("trimming a %s device brought its per-FAT sector count back within the uint16 go-diskfs records, hiding a size that must be refused",
				GibibytesString(size))
		}
	}
}
