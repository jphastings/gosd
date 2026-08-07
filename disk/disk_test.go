package disk

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/jphastings/gosd/internal/blockmount"
	"github.com/jphastings/gosd/internal/diskfmt"
)

// present is a disk that is attached, has a medium and is writable — the
// baseline every discovery case varies from.
func present(name string, partitions ...string) blockmount.Device {
	return blockmount.Device{Name: name, SizeSectors: 1 << 20, Partitions: partitions}
}

func TestChoose(t *testing.T) {
	for _, tc := range []struct {
		name    string
		devices []blockmount.Device
		mounted map[string]bool
		want    string
	}{{
		name:    "an NVMe SSD alongside the boot microSD",
		devices: []blockmount.Device{present("mmcblk0", "mmcblk0p1", "mmcblk0p2"), present("nvme0n1")},
		mounted: map[string]bool{"/dev/mmcblk0p1": true, "/dev/mmcblk0p2": true},
		want:    "/dev/nvme0n1",
	}, {
		name:    "a USB drive alongside the boot microSD",
		devices: []blockmount.Device{present("mmcblk0", "mmcblk0p1"), present("sda", "sda1")},
		mounted: map[string]bool{"/dev/mmcblk0p1": true},
		want:    "/dev/sda",
	}, {
		name:    "NVMe wins over a USB drive and an idle eMMC",
		devices: []blockmount.Device{present("sda"), present("mmcblk1"), present("nvme0n1")},
		want:    "/dev/nvme0n1",
	}, {
		name:    "a USB drive wins over an idle eMMC",
		devices: []blockmount.Device{present("mmcblk1"), present("sdb"), present("sda")},
		want:    "/dev/sda",
	}, {
		name:    "a virtio disk that is not the boot device",
		devices: []blockmount.Device{present("vda", "vda1"), present("vdb")},
		mounted: map[string]bool{"/dev/vda1": true},
		want:    "/dev/vdb",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := choose(tc.devices, tc.mounted)
			if err != nil {
				t.Fatalf("choose: %v", err)
			}
			if got != tc.want {
				t.Errorf("choose = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestChooseReportsErrNoDisk(t *testing.T) {
	for _, tc := range []struct {
		name    string
		devices []blockmount.Device
		mounted map[string]bool
	}{{
		name: "nothing attached at all",
	}, {
		name:    "the only disk is the one we booted from",
		devices: []blockmount.Device{present("nvme0n1", "nvme0n1p1")},
		mounted: map[string]bool{"/dev/nvme0n1p1": true},
	}, {
		name:    "an empty card-reader slot reports no medium",
		devices: []blockmount.Device{{Name: "sdb", SizeSectors: 0}},
	}, {
		name:    "a write-protected card cannot be formatted",
		devices: []blockmount.Device{{Name: "sdc", SizeSectors: 1 << 20, ReadOnly: true}},
	}, {
		name: "virtual and non-mass-storage nodes are never format targets",
		devices: []blockmount.Device{
			present("loop0"),        // a file, not media
			present("ram0"),         // volatile
			present("zram0"),        // volatile, compressed
			present("zd0"),          // a zvol
			present("dm-0"),         // device-mapper: formatting corrupts what it maps
			present("md0"),          // an MD RAID array
			present("sr0"),          // optical
			present("nbd0"),         // a network block device
			present("mtdblock0"),    // raw flash translation
			present("ubiblock0"),    // ditto
			present("mmcblk0boot0"), // eMMC hardware boot partition
			present("mmcblk0boot1"), // eMMC hardware boot partition
			present("mmcblk0rpmb"),  // eMMC replay-protected block
			present("mmcblk0gp0"),   // eMMC general-purpose hardware partition
			present("mmcblk0gp1"),   // eMMC general-purpose hardware partition
			present("mmcblk0gp2"),   // eMMC general-purpose hardware partition
			present("mmcblk0gp3"),   // eMMC general-purpose hardware partition
		},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := choose(tc.devices, tc.mounted); !errors.Is(err, ErrNoDisk) {
				t.Fatalf("choose error = %v, want ErrNoDisk", err)
			}
		})
	}
}

// TestChooseNeverPicksAnEMMCGeneralPurposeHardwarePartition pins the exact
// failure scenario gosd-f226 found: a Rockchip board booted from its eMMC (so
// mmcblk0 itself is in use and already excluded), whose vendor-configured GP
// hardware partition — its own /sys/block gendisk, with a medium and no
// size==0/ReadOnly signal to catch it — must still never become the chosen
// format target, or FormatAndMount would wipe DRM keys/calibration/secure
// storage with no way back.
func TestChooseNeverPicksAnEMMCGeneralPurposeHardwarePartition(t *testing.T) {
	devices := []blockmount.Device{
		present("mmcblk0", "mmcblk0p1"),
		present("mmcblk0gp0"),
	}
	mounted := map[string]bool{"/dev/mmcblk0p1": true}

	if _, err := choose(devices, mounted); !errors.Is(err, ErrNoDisk) {
		t.Fatalf("choose error = %v, want ErrNoDisk", err)
	}
	if got := candidates(devices, mounted); len(got) != 0 {
		t.Errorf("candidates = %v, want none — the GP partition must never be listed", got)
	}
}

// TestRankLeavesMediumAndWriteProtectionToBlockmount pins the responsibility
// split gosd-ix38 introduced: rank now expresses only disk's own class
// preference and its eMMC-hardware-partition exclusion, so a device with no
// medium or that is write-protected still ranks as acceptable here — it is
// blockmount.Usable, applied inside Candidates/Choose, that keeps such a
// device off the list (see TestChooseReportsErrNoDisk for the end-to-end
// behaviour, unchanged by this split).
func TestRankLeavesMediumAndWriteProtectionToBlockmount(t *testing.T) {
	for _, dev := range []blockmount.Device{
		{Name: "sda", SizeSectors: 0},
		{Name: "sdb", SizeSectors: 1 << 20, ReadOnly: true},
	} {
		if _, ok := rank(dev); !ok {
			t.Errorf("rank(%+v) ok = false, want true — medium/write-protection filtering belongs to blockmount.Usable now, not rank", dev)
		}
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
			if got := isMMCHardwarePartition(tc.name); got != tc.want {
				t.Errorf("isMMCHardwarePartition(%q) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

func TestCandidatesListsEveryUsableDiskBestFirst(t *testing.T) {
	devices := []blockmount.Device{
		present("mmcblk0", "mmcblk0p1"), // the boot microSD
		present("mmcblk1"),              // an idle eMMC
		present("loop0"),
		present("sdb"),
		present("sda"),
		present("nvme0n1"),
	}
	mounted := map[string]bool{"/dev/mmcblk0p1": true}

	got := candidates(devices, mounted)
	want := []string{"/dev/nvme0n1", "/dev/sda", "/dev/sdb", "/dev/mmcblk1"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("candidates = %v, want %v", got, want)
	}
}

func TestVerifyNamedRefusesADiskInUse(t *testing.T) {
	// Naming a device explicitly skips the class allowlist but never the in-use
	// rule — otherwise a typo could reformat the running system.
	devices := []blockmount.Device{present("mmcblk0", "mmcblk0p1"), present("nvme0n1")}
	mounted := map[string]bool{"/dev/mmcblk0p1": true}

	_, err := verifyNamed("/dev/mmcblk0", devices, mounted)
	if err == nil {
		t.Fatal("verifyNamed accepted the device the board booted from")
	}
	if !strings.Contains(err.Error(), "/dev/nvme0n1") {
		t.Errorf("error = %q, want it to point at the usable alternative", err)
	}
}

func TestVerifyNamedAcceptsAnIdleDisk(t *testing.T) {
	devices := []blockmount.Device{present("mmcblk0", "mmcblk0p1"), present("sda")}
	mounted := map[string]bool{"/dev/mmcblk0p1": true}

	got, err := verifyNamed("/dev/sda", devices, mounted)
	if err != nil {
		t.Fatalf("verifyNamed: %v", err)
	}
	if got != "/dev/sda" {
		t.Errorf("verifyNamed = %q, want /dev/sda", got)
	}
}

func TestVerifyNamedReportsErrNoDiskForAnAbsentDevice(t *testing.T) {
	_, err := verifyNamed("/dev/nvme9n9", []blockmount.Device{present("sda")}, nil)

	if !errors.Is(err, ErrNoDisk) {
		t.Fatalf("verifyNamed error = %v, want ErrNoDisk", err)
	}
}

func TestFormatAndMountSurfacesErrRefusedFormat(t *testing.T) {
	// The realistic NVMe case: the drive arrives carrying somebody else's
	// exFAT volume, so it is refused rather than silently wiped — and the error
	// says what is on it.
	deps := blockmount.Deps{
		MountedAt: func(string) (string, bool, error) { return "", false, nil },
		Discover:  func() (string, error) { return "/dev/nvme0n1", nil },
		Inspect: func(string) (diskfmt.Contents, error) {
			return diskfmt.Contents{FS: diskfmt.ExFAT, Label: "SOMEONELSE"}, nil
		},
		Format: func(string, string, diskfmt.FS) error {
			t.Fatal("formatted a disk holding another filesystem")
			return nil
		},
		Mount:     func(string, string, diskfmt.FS) error { return nil },
		Mountable: func(diskfmt.FS) (bool, error) { return true, nil },
	}

	_, err := blockmount.Run(storage(deps), diskfmt.FAT32, "APPDATA", "/storage", false)

	if !errors.Is(err, ErrRefusedFormat) {
		t.Fatalf("error = %v, want ErrRefusedFormat", err)
	}
	if !strings.Contains(err.Error(), "the disk at /dev/nvme0n1") || !strings.Contains(err.Error(), "exFAT") {
		t.Errorf("error = %q, want it to name the disk, its device node and the filesystem found", err)
	}
}

func TestLabelErrorsAreAttributedToThisPackage(t *testing.T) {
	// "APPDATA " has a trailing space, and "ABCDEFG H" has a space at FAT's
	// short-name/extension split (byte 7): both provably cannot round-trip
	// through format→Inspect, which without this check reformats — and
	// destroys — the app's own data on every boot.
	for _, label := range []string{"WAYTOOLONGFORFAT", "APPDATA ", "ABCDEFG H"} {
		_, err := blockmount.Run(storage(blockmount.Deps{}), diskfmt.FAT32, label, "/storage", false)

		if err == nil || !strings.HasPrefix(err.Error(), "disk: ") {
			t.Fatalf("error for %q = %v, want a disk-prefixed label complaint", label, err)
		}
	}
}

// TestOptionsZeroValueIsTheEXT4Default pins the epic's locked, deliberately
// breaking default (gosd-lfu0/gosd-1c0x): an empty Options — and FormatAndMount
// itself, which passes one — formats ext4, not FAT32. FAT32 and exFAT remain
// reachable as explicit choices, for removable media meant to be read on
// another host.
func TestOptionsZeroValueIsTheEXT4Default(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts Options
		want diskfmt.FS
	}{
		{"zero value", Options{}, diskfmt.EXT4},
		{"EXT4 spelled out", Options{Filesystem: EXT4}, diskfmt.EXT4},
		{"FAT32", Options{Filesystem: FAT32}, diskfmt.FAT32},
		{"exFAT", Options{Filesystem: ExFAT}, diskfmt.ExFAT},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.opts.filesystem()
			if err != nil {
				t.Fatalf("filesystem(): %v", err)
			}
			if got != tc.want {
				t.Errorf("filesystem() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestFormatAndMountWithRejectsAnUnknownFilesystem proves the bad-option path
// still honours the channel contract — exactly one Result, then closed — and
// that it says which value was unusable.
func TestFormatAndMountWithRejectsAnUnknownFilesystem(t *testing.T) {
	ch := FormatAndMountWith("APPDATA", "/storage", Options{Filesystem: "ntfs"})

	res := <-ch
	if res.Err == nil || !strings.Contains(res.Err.Error(), "ntfs") {
		t.Fatalf("Err = %v, want it to name the unusable filesystem", res.Err)
	}
	if _, open := <-ch; open {
		t.Error("the channel delivered more than one Result")
	}
}
