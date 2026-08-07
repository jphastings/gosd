package emmc

import (
	"errors"
	"strings"
	"testing"

	"github.com/jphastings/gosd/internal/blockmount"
	"github.com/jphastings/gosd/internal/diskfmt"
)

// fakeDeps scripts the platform operations so the package's own wiring — its
// noun, its label-error prefix and its sentinels — can be exercised without a
// real eMMC. The mount-only / format / refuse decision itself is shared and
// tested in internal/blockmount.
func fakeDeps(contents diskfmt.Contents, discErr error) blockmount.Deps {
	return blockmount.Deps{
		MountedAt: func(string) (string, bool, error) { return "", false, nil },
		Discover: func() (string, error) {
			if discErr != nil {
				return "", discErr
			}
			return "/dev/mmcblk1", nil
		},
		Inspect:   func(string) (diskfmt.Contents, error) { return contents, nil },
		Format:    func(string, string, diskfmt.FS) error { return nil },
		Mount:     func(string, string, diskfmt.FS) error { return nil },
		Mountable: func(diskfmt.FS) (bool, error) { return true, nil },
	}
}

func TestFormatAndMountSurfacesErrNoEMMC(t *testing.T) {
	_, err := blockmount.Run(storage(fakeDeps(diskfmt.Contents{}, ErrNoEMMC)), diskfmt.FAT32, "APPDATA", "/storage", false)

	if !errors.Is(err, ErrNoEMMC) {
		t.Fatalf("error = %v, want ErrNoEMMC", err)
	}
}

func TestFormatAndMountSurfacesErrRefusedFormat(t *testing.T) {
	deps := fakeDeps(diskfmt.Contents{FS: diskfmt.FAT32, Label: "OTHERAPP"}, nil)

	_, err := blockmount.Run(storage(deps), diskfmt.FAT32, "APPDATA", "/storage", false)

	if !errors.Is(err, ErrRefusedFormat) {
		t.Fatalf("error = %v, want ErrRefusedFormat", err)
	}
	if !strings.Contains(err.Error(), "the eMMC at /dev/mmcblk1") {
		t.Errorf("error = %q, want it to name the eMMC and its device node", err)
	}
}

func TestLabelErrorsAreAttributedToThisPackage(t *testing.T) {
	// "APPDATA " has a trailing space, and "ABCDEFG H" has a space at FAT's
	// short-name/extension split (byte 7): both provably cannot round-trip
	// through format→Inspect, which without this check reformats — and
	// destroys — the app's own data on every boot.
	for _, label := range []string{"WAYTOOLONGFORFAT", "APPDATA ", "ABCDEFG H"} {
		_, err := blockmount.Run(storage(fakeDeps(diskfmt.Contents{}, nil)), diskfmt.FAT32, label, "/storage", false)

		if err == nil || !strings.HasPrefix(err.Error(), "emmc: ") {
			t.Fatalf("error for %q = %v, want an emmc-prefixed label complaint", label, err)
		}
	}
}

// present is an eMMC that is attached, has a medium and is writable — the
// baseline every discovery case varies from.
func present(name, kind string, partitions ...string) blockmount.Device {
	return blockmount.Device{Name: name, Kind: kind, SizeSectors: 1 << 20, Partitions: partitions}
}

func TestChooseEMMCPrefersUnmountedMMCRegardlessOfNumber(t *testing.T) {
	// The eMMC is mmcblk1 here and the booted SD is mmcblk0, proving selection
	// is by type + not-in-use, not by device number.
	devices := []blockmount.Device{
		present("mmcblk0", "SD", "mmcblk0p1", "mmcblk0p2"),
		present("mmcblk1", "MMC"),
	}
	mounted := map[string]bool{"/dev/mmcblk0p1": true, "/dev/mmcblk0p2": true}

	got, err := chooseEMMC(devices, mounted)
	if err != nil {
		t.Fatalf("chooseEMMC: %v", err)
	}
	if got != "/dev/mmcblk1" {
		t.Errorf("chooseEMMC = %q, want /dev/mmcblk1", got)
	}
}

func TestChooseEMMCSkipsTheBootDevice(t *testing.T) {
	// Booting from the eMMC: its partitions are mounted, so it must be off
	// limits and discovery must report no eMMC rather than a wiped system.
	devices := []blockmount.Device{
		{Name: "mmcblk0", Kind: "MMC", Partitions: []string{"mmcblk0p1", "mmcblk0p2"}},
	}
	mounted := map[string]bool{"/dev/mmcblk0p1": true, "/dev/mmcblk0p2": true}

	if _, err := chooseEMMC(devices, mounted); !errors.Is(err, ErrNoEMMC) {
		t.Fatalf("chooseEMMC error = %v, want ErrNoEMMC", err)
	}
}

func TestChooseEMMCIgnoresNonMMCStorage(t *testing.T) {
	// An SD card, an NVMe SSD and a USB drive are all mass storage, but none of
	// them is the onboard eMMC — they belong to the disk package.
	devices := []blockmount.Device{
		{Name: "mmcblk0", Kind: "SD", Partitions: []string{"mmcblk0p1"}},
		{Name: "nvme0n1"},
		{Name: "sda"},
	}

	if _, err := chooseEMMC(devices, nil); !errors.Is(err, ErrNoEMMC) {
		t.Fatalf("chooseEMMC error = %v, want ErrNoEMMC", err)
	}
}

// TestChooseEMMCIgnoresGeneralPurposeHardwarePartitions pins the sysfs quirk
// chooseEMMC's doc comment relies on (see gosd-f226/gosd-ix38): a GP hardware
// partition's gendisk reports no device/type, so Kind is "" rather than
// "MMC" — accidentally, not by an explicit exclusion like disk.rank's. If a
// future kernel or sysfs change ever gave these Kind == "MMC", this test
// would start failing and say exactly why.
func TestChooseEMMCIgnoresGeneralPurposeHardwarePartitions(t *testing.T) {
	devices := []blockmount.Device{
		{Name: "mmcblk0", Kind: "MMC", Partitions: []string{"mmcblk0p1"}},
		{Name: "mmcblk0gp0", Kind: ""},
		{Name: "mmcblk0gp1", Kind: ""},
	}
	mounted := map[string]bool{"/dev/mmcblk0p1": true}

	if _, err := chooseEMMC(devices, mounted); !errors.Is(err, ErrNoEMMC) {
		t.Fatalf("chooseEMMC error = %v, want ErrNoEMMC", err)
	}
}

// TestChooseEMMCRejectsNoMediumOrWriteProtected pins gosd-ix38's fix.
// chooseEMMC's rank is `dev.Kind == "MMC"` alone — before this fix, an
// MMC-typed device with no medium or one that was write-protected would have
// been picked as a format target, exactly the divergence from disk.rank
// (which always rejected both) that the bean found. blockmount.Usable now
// rejects both for every caller of Choose/Candidates, so emmc inherits the
// same rule as disk without rank having to spell it out.
func TestChooseEMMCRejectsNoMediumOrWriteProtected(t *testing.T) {
	for _, tc := range []struct {
		name string
		dev  blockmount.Device
	}{
		{"no medium", blockmount.Device{Name: "mmcblk1", Kind: "MMC", SizeSectors: 0}},
		{"write-protected", blockmount.Device{Name: "mmcblk1", Kind: "MMC", SizeSectors: 1 << 20, ReadOnly: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := chooseEMMC([]blockmount.Device{tc.dev}, nil); !errors.Is(err, ErrNoEMMC) {
				t.Fatalf("chooseEMMC error = %v, want ErrNoEMMC", err)
			}
		})
	}
}
