package blockmount

import (
	"errors"
	"reflect"
	"testing"
)

// acceptAll ranks every device equally, so these tests isolate the shared
// rules — in-use exclusion and ordering — from any package's class allowlist.
func acceptAll(Device) (int, bool) { return 0, true }

// present is a device that is attached, has a medium and is writable — the
// baseline every discovery case in this file varies from, so tests that are
// not themselves about Usable don't need to restate it.
func present(name string, partitions ...string) Device {
	return Device{Name: name, SizeSectors: 1 << 20, Partitions: partitions}
}

func TestCandidatesExcludesAnythingMountedFromTheDevice(t *testing.T) {
	// The whole point of the rule: the media the board booted from has a
	// mounted partition, so it must never be offered as a format target.
	devices := []Device{
		present("mmcblk0", "mmcblk0p1", "mmcblk0p2"),
		present("nvme0n1"),
		present("sda"),
	}
	mounted := map[string]bool{"/dev/mmcblk0p1": true, "/dev/sda": true}

	got := Candidates(devices, mounted, acceptAll)
	if want := []string{"/dev/nvme0n1"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Candidates = %v, want %v (boot media and a whole-device mount both excluded)", got, want)
	}
}

func TestCandidatesOrderIsIndependentOfEnumerationOrder(t *testing.T) {
	rank := func(dev Device) (int, bool) {
		if dev.Name == "nvme0n1" {
			return 0, true
		}
		return 1, true
	}
	devices := []Device{present("sdb"), present("sda"), present("nvme0n1")}

	got := Candidates(devices, nil, rank)
	if want := []string{"/dev/nvme0n1", "/dev/sda", "/dev/sdb"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Candidates = %v, want %v (rank first, then name)", got, want)
	}
}

func TestChooseReportsTheCallersSentinelWhenNothingQualifies(t *testing.T) {
	none := errors.New("nothing here")

	if _, err := Choose(nil, nil, acceptAll, none); !errors.Is(err, none) {
		t.Fatalf("Choose error = %v, want the caller's sentinel", err)
	}
}

// TestCandidatesExcludesNoMediumAndWriteProtectedRegardlessOfRank pins
// gosd-ix38's fix at the level both emmc and disk share: acceptAll would
// happily rank a no-medium or write-protected device, but Candidates must
// never offer one as a format target, so this check cannot depend on any
// individual package's Rank remembering to make it — see Usable's doc.
func TestCandidatesExcludesNoMediumAndWriteProtectedRegardlessOfRank(t *testing.T) {
	devices := []Device{
		{Name: "sda", SizeSectors: 1 << 20},
		{Name: "sdb", SizeSectors: 0},                       // an empty card-reader slot
		{Name: "sdc", SizeSectors: 1 << 20, ReadOnly: true}, // write-protected
	}

	got := Candidates(devices, nil, acceptAll)
	if want := []string{"/dev/sda"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Candidates = %v, want %v (no-medium and write-protected devices excluded even though acceptAll ranks everything)", got, want)
	}
}

func TestUsable(t *testing.T) {
	for _, tc := range []struct {
		name string
		dev  Device
		want bool
	}{
		{"present and writable", Device{SizeSectors: 1 << 20}, true},
		{"no medium", Device{SizeSectors: 0}, false},
		{"write-protected", Device{SizeSectors: 1 << 20, ReadOnly: true}, false},
		{"no medium and write-protected", Device{SizeSectors: 0, ReadOnly: true}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Usable(tc.dev); got != tc.want {
				t.Errorf("Usable(%+v) = %v, want %v", tc.dev, got, tc.want)
			}
		})
	}
}
