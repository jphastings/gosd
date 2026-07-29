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
			present("mmcblk0rpmb"),  // eMMC replay-protected block
		},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := choose(tc.devices, tc.mounted); !errors.Is(err, ErrNoDisk) {
				t.Fatalf("choose error = %v, want ErrNoDisk", err)
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
	// The realistic NVMe case: the drive arrives exFAT-formatted, which GoSD
	// cannot mount, so it is refused rather than silently wiped — and the error
	// says what is on it.
	deps := blockmount.Deps{
		MountedAt: func(string) (string, bool, error) { return "", false, nil },
		Discover:  func() (string, error) { return "/dev/nvme0n1", nil },
		Inspect:   func(string) (diskfmt.Contents, error) { return diskfmt.Contents{OtherFS: "exFAT"}, nil },
		Format:    func(string, string) error { t.Fatal("formatted a disk holding another filesystem"); return nil },
		Mount:     func(string, string) error { return nil },
	}

	_, err := blockmount.Run(storage(deps), "APPDATA", "/storage", false)

	if !errors.Is(err, ErrRefusedFormat) {
		t.Fatalf("error = %v, want ErrRefusedFormat", err)
	}
	if !strings.Contains(err.Error(), "the disk at /dev/nvme0n1") || !strings.Contains(err.Error(), "exFAT") {
		t.Errorf("error = %q, want it to name the disk, its device node and the filesystem found", err)
	}
}

func TestLabelErrorsAreAttributedToThisPackage(t *testing.T) {
	_, err := blockmount.Run(storage(blockmount.Deps{}), "WAYTOOLONGFORFAT", "/storage", false)

	if err == nil || !strings.HasPrefix(err.Error(), "disk: ") {
		t.Fatalf("error = %v, want a disk-prefixed label complaint", err)
	}
}
