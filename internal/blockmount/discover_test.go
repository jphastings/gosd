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
		{"present and writable", Device{Name: "sda", SizeSectors: 1 << 20}, true},
		{"no medium", Device{Name: "sda", SizeSectors: 0}, false},
		{"write-protected", Device{Name: "sda", SizeSectors: 1 << 20, ReadOnly: true}, false},
		{"no medium and write-protected", Device{Name: "sda", SizeSectors: 0, ReadOnly: true}, false},
		{"eMMC boot hardware partition", Device{Name: "mmcblk0boot0", SizeSectors: 1 << 20}, false},
		{"eMMC RPMB hardware partition", Device{Name: "mmcblk0rpmb", SizeSectors: 1 << 20}, false},
		{"eMMC GP hardware partition", Device{Name: "mmcblk0gp0", SizeSectors: 1 << 20}, false},
		{"the eMMC user area itself", Device{Name: "mmcblk0", SizeSectors: 1 << 20}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Usable(tc.dev); got != tc.want {
				t.Errorf("Usable(%+v) = %v, want %v", tc.dev, got, tc.want)
			}
		})
	}
}

// TestCandidatesExcludesMMCHardwarePartitionsRegardlessOfRank pins the
// invariant bean gosd-b6jl relies on, and the reason the exclusion moved here
// from disk: Usable is consulted for every caller of Candidates/Choose,
// before that caller's Rank is ever asked, so neither public package can
// offer an eMMC's boot/RPMB/GP hardware partition however permissive its own
// Rank is. acceptAll is as permissive as a Rank gets — it stands in for
// emmc's, which before this bean accepted anything reporting Kind == "MMC"
// and stayed clear of these devices only because sysfs happens not to give
// them a device/type. Formatting boot0 leaves a board that no longer boots
// and cannot be recovered from the SD card, so this must not depend on a
// kernel behaviour nobody has verified.
func TestCandidatesExcludesMMCHardwarePartitionsRegardlessOfRank(t *testing.T) {
	devices := []Device{
		present("mmcblk0"),
		present("mmcblk0boot0"),
		present("mmcblk0boot1"),
		present("mmcblk0rpmb"),
		present("mmcblk0gp0"),
		present("mmcblk0p1"), // an ordinary partition name must not be caught by mistake
	}

	got := Candidates(devices, nil, acceptAll)
	if want := []string{"/dev/mmcblk0", "/dev/mmcblk0p1"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Candidates = %v, want %v (every hardware partition excluded, the user area and a plain partition kept)", got, want)
	}
}

// TestIsMMCHardwarePartition proves the matcher is structural (a regex
// anchored on the kernel's actual naming), not a suffix list: it must reject
// every hardware-partition shape the MMC block driver creates without also
// rejecting a plain device or one of its ordinary partitions.
func TestIsMMCHardwarePartition(t *testing.T) {
	for _, tc := range []struct {
		name string
		want bool
	}{
		// The kernel's real hardware-partition gendisks.
		{"mmcblk0boot0", true},
		{"mmcblk0boot1", true},
		{"mmcblk0rpmb", true},
		{"mmcblk0gp0", true},
		{"mmcblk0gp1", true},
		{"mmcblk0gp2", true},
		{"mmcblk0gp3", true},
		// A double-digit device number must not change the outcome.
		{"mmcblk10boot0", true},
		{"mmcblk10rpmb", true},
		{"mmcblk10gp0", true},
		// Plain devices and their ordinary partitions must never be rejected —
		// a suffix check on "p1" or a bare device number risks exactly this
		// false positive.
		{"mmcblk0", false},
		{"mmcblk10", false},
		{"mmcblk0p1", false},
		{"mmcblk0p10", false},
		{"mmcblk10p1", false},
		{"nvme0n1", false},
		{"sda", false},
		// Adversarial/defensive shapes: an index the kernel doesn't use today
		// (gp only ever goes 0-3) is still the GP hardware-partition class, so
		// it is rejected rather than trusted to never occur.
		{"mmcblk0gp10", true},
		// Shapes that merely contain the words, but aren't the kernel's naming
		// at all, must not match.
		{"mmcblk0gpx", false},
		{"mmcblkboot0", false},
		{"mmcblk0bootx", false},
		{"mmcblk0rpmbx", false},
		{"xmmcblk0boot0", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsMMCHardwarePartition(tc.name); got != tc.want {
				t.Errorf("IsMMCHardwarePartition(%q) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}
