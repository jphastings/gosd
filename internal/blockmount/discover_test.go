package blockmount

import (
	"errors"
	"reflect"
	"testing"
)

// acceptAll ranks every device equally, so these tests isolate the shared
// rules — in-use exclusion and ordering — from any package's class allowlist.
func acceptAll(Device) (int, bool) { return 0, true }

func TestCandidatesExcludesAnythingMountedFromTheDevice(t *testing.T) {
	// The whole point of the rule: the media the board booted from has a
	// mounted partition, so it must never be offered as a format target.
	devices := []Device{
		{Name: "mmcblk0", Partitions: []string{"mmcblk0p1", "mmcblk0p2"}},
		{Name: "nvme0n1"},
		{Name: "sda"},
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
	devices := []Device{{Name: "sdb"}, {Name: "sda"}, {Name: "nvme0n1"}}

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
