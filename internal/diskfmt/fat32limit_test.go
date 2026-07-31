package diskfmt

import (
	"strings"
	"testing"
)

// The boundary, from the arithmetic go-diskfs lays a FAT32 volume out with: a
// volume this large gets 32 KiB clusters, which cost 32768 bytes of data plus
// 8 bytes of entry across the two FATs, so 65535 sectors per FAT reach
// 65535*32776/4 + 32 reserved = 536,993,822 sectors. One sector more needs a
// 65536th FAT sector, which go-diskfs's uint16 cannot hold.
const (
	largestSafeFAT32Sectors = 536_993_822
	largestSafeFAT32Bytes   = largestSafeFAT32Sectors * sectorSizeBytes // 274,940,836,864, a shade over 256 GiB
)

func TestFAT32SizeLimitSitsWhereSectorsPerFATOverflows(t *testing.T) {
	if maxFAT32Bytes != largestSafeFAT32Bytes {
		t.Errorf("FAT32 size limit = %d bytes (%s), want %d bytes (%s)",
			maxFAT32Bytes, GibibytesString(maxFAT32Bytes), int64(largestSafeFAT32Bytes), GibibytesString(largestSafeFAT32Bytes))
	}
	if got := fat32SectorsPerFAT(largestSafeFAT32Bytes); got != fat32MaxSectorsPerFAT {
		t.Errorf("sectors per FAT at the limit = %d, want %d (the largest go-diskfs can record)", got, fat32MaxSectorsPerFAT)
	}
	if got := fat32SectorsPerFAT(largestSafeFAT32Bytes + sectorSizeBytes); got != fat32MaxSectorsPerFAT+1 {
		t.Errorf("sectors per FAT one sector past the limit = %d, want %d (one more than go-diskfs can record)", got, fat32MaxSectorsPerFAT+1)
	}
}

func TestCheckFAT32SizeRefusesOnlyVolumesItCannotLayOut(t *testing.T) {
	for _, tc := range []struct {
		name    string
		size    int64
		refused bool
	}{
		{"a 64 GiB SD card", 64 << 30, false},
		{"256 GiB, the size dataexpand caps a grown partition at", 256 << 30, false},
		{"the largest volume that still fits", largestSafeFAT32Bytes, false},
		{"one sector past it", largestSafeFAT32Bytes + sectorSizeBytes, true},
		{"a 512 GB SSD", 512_000_000_000, true},
		{"a 1 TB USB drive", 1_000_204_886_016, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := checkFAT32Size("/dev/sda", tc.size)
			if (err != nil) != tc.refused {
				t.Fatalf("checkFAT32Size(%d) error = %v, want refused = %v", tc.size, err, tc.refused)
			}
			if err == nil {
				return
			}
			// The refusal has to leave the app author somewhere to go.
			for _, want := range []string{"/dev/sda", GibibytesString(maxFAT32Bytes), "exFAT"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("refusal %q does not mention %q", err, want)
				}
			}
		})
	}
}

// TestFormatFAT32RefusesTooLargeADeviceWithoutTouchingIt is the point of the
// guard: a drive big enough to overflow go-diskfs's sectors-per-FAT count used
// to be formatted into a corrupt filesystem with no error at all.
func TestFormatFAT32RefusesTooLargeADeviceWithoutTouchingIt(t *testing.T) {
	path := backingFile(t, 512_000_000_000) // sparse: a 512 GB SSD costs no bytes here

	err := FormatFAT32(path, "BIGDISK")
	if err == nil {
		t.Fatal("FormatFAT32 of a 512 GB device succeeded; want a refusal, since the filesystem it writes would be corrupt")
	}
	if !strings.Contains(err.Error(), "exFAT") {
		t.Errorf("refusal %q does not point at exFAT", err)
	}

	contents, err := Inspect(path)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if !contents.Blank {
		t.Errorf("device after the refusal = %+v, want it untouched (blank)", contents)
	}
}
