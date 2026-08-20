package emmc

import (
	"errors"
	"reflect"
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

// TestChooseEMMCNeverPicksAHardwarePartition is bean gosd-b6jl's regression
// test. Every one of these devices reports Kind == "MMC" — the state the
// kernel does NOT put them in today, which is the only reason chooseEMMC's
// `Kind == "MMC"` rank stayed clear of them before this bean. That protection
// was an undocumented sysfs quirk, never verified on the Rockchip boards GoSD
// ships, and silent if it ever regressed: formatting boot0/boot1 leaves a
// board that no longer boots and cannot be recovered from the SD card, and a
// GP area typically holds vendor keys or calibration data. They are now
// excluded by name, in blockmount.Usable and in this rank, so a kernel that
// started populating device/type for them would change nothing.
func TestChooseEMMCNeverPicksAHardwarePartition(t *testing.T) {
	for _, name := range []string{"mmcblk0boot0", "mmcblk0boot1", "mmcblk0rpmb", "mmcblk0gp0", "mmcblk0gp3"} {
		t.Run(name, func(t *testing.T) {
			devices := []blockmount.Device{
				present("mmcblk0", "MMC", "mmcblk0p1"), // the eMMC itself: booted from, so in use
				present(name, "MMC"),
			}
			mounted := map[string]bool{"/dev/mmcblk0p1": true}

			if _, err := chooseEMMC(devices, mounted); !errors.Is(err, ErrNoEMMC) {
				t.Fatalf("chooseEMMC = %v, want ErrNoEMMC — %s must never be a format target", err, name)
			}
		})
	}
}

// TestChooseEMMCStillPicksTheUserArea is the other half: the exclusion is by
// the kernel's hardware-partition naming, so it must not catch the eMMC's own
// user-data device or an ordinary partition name.
func TestChooseEMMCStillPicksTheUserArea(t *testing.T) {
	devices := []blockmount.Device{present("mmcblk1", "MMC")}

	got, err := chooseEMMC(devices, nil)
	if err != nil {
		t.Fatalf("chooseEMMC: %v", err)
	}
	if got != "/dev/mmcblk1" {
		t.Errorf("chooseEMMC = %q, want /dev/mmcblk1", got)
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

// --- ext4: emmc mirrors disk/blockmount's shared state machine (bean
// gosd-9sc4, superseding #192-era "emmc is FAT32-only / ext4 unreachable
// from emmc" assumptions — see gosd-1c0x's Summary of Changes, which this
// test set mirrors emmc-flavored) ---

// TestOptionsZeroValueIsTheEXT4Default pins this bean's locked, deliberately
// breaking default: an empty Options — and FormatAndMount itself, which
// passes one — formats ext4, not FAT32. FAT32 and exFAT remain reachable as
// explicit choices. Mirrors disk_test.go's identically-named test.
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
// that it says which value was unusable. This runs the real
// FormatAndMountWith (not blockmount.Run against a fake): opts.filesystem()
// is checked before newPlatformDeps is ever touched, so it works on every
// host, not just Linux.
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

// ext4Fake scripts the platform operations for the establishment/
// adoption/crash-debris/mismatch tests below, mirroring
// internal/blockmount's own fakeDeps (bean gosd-1c0x) but driven through
// emmc's own storage() helper (Pkg: "emmc", Noun: "eMMC") rather than a
// generic "fakepkg"/"widget" one — these tests exist to prove emmc's own
// wiring reaches the shared runEXT4 machinery correctly, not to re-litigate
// runEXT4's internal logic, which internal/blockmount/blockmount_test.go
// already covers exhaustively and package-agnostically.
type ext4Fake struct {
	contents      diskfmt.Contents
	markerPresent bool
	rootHasOther  bool
	unmountable   diskfmt.FS

	calls []string
}

func (f *ext4Fake) deps() blockmount.Deps {
	return blockmount.Deps{
		MountedAt: func(string) (string, bool, error) { return "", false, nil },
		Discover:  func() (string, error) { return "/dev/mmcblk1", nil },
		Inspect: func(string) (diskfmt.Contents, error) {
			f.calls = append(f.calls, "inspect")
			return f.contents, nil
		},
		Format: func(_, label string, fs diskfmt.FS) error {
			f.calls = append(f.calls, "format")
			f.contents = diskfmt.Contents{FS: fs, Label: label}
			return nil
		},
		Mount: func(string, string, diskfmt.FS) error {
			f.calls = append(f.calls, "mount")
			return nil
		},
		Mountable:      func(fs diskfmt.FS) (bool, error) { return fs != f.unmountable, nil },
		MountedSources: func() (map[string]bool, error) { return map[string]bool{}, nil },
		SyncDevice: func(string) error {
			f.calls = append(f.calls, "sync")
			return nil
		},
		Grow: func(string, string) error {
			f.calls = append(f.calls, "grow")
			return nil
		},
		EstablishMarker: func(_ string, _ diskfmt.FS) error {
			f.calls = append(f.calls, "marker")
			f.markerPresent = true
			return nil
		},
		MarkerEstablished: func(string) (bool, error) {
			f.calls = append(f.calls, "check-marker")
			return f.markerPresent, nil
		},
		RootHasOtherContent: func(_ string, _ diskfmt.FS) (bool, error) {
			f.calls = append(f.calls, "check-root")
			return f.rootHasOther, nil
		},
		Unmount: func(string) error {
			f.calls = append(f.calls, "unmount")
			return nil
		},
	}
}

// TestRunEXT4FreshFormatEstablishesInOrder pins the crash-safe establishment
// sequence's exact call order on a blank eMMC — format, sync, mount, grow,
// marker, last of all — reached through emmc's storage(), mirroring
// internal/blockmount's identical disk-flavored test.
func TestRunEXT4FreshFormatEstablishesInOrder(t *testing.T) {
	f := &ext4Fake{contents: diskfmt.Contents{Blank: true}}

	device, err := blockmount.Run(storage(f.deps()), diskfmt.EXT4, "APPDATA", "/storage", false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if device != "/dev/mmcblk1" {
		t.Errorf("Run device = %q, want /dev/mmcblk1", device)
	}
	want := []string{"inspect", "format", "sync", "mount", "grow", "marker"}
	if !reflect.DeepEqual(f.calls, want) {
		t.Errorf("call order = %v, want %v", f.calls, want)
	}
}

// TestRunEXT4AdoptsAnEstablishedVolumeWithoutFormatOrGrow is the adoption
// half: a matching label and filesystem, plus the establishment marker,
// mounts as-is — no format, no grow.
func TestRunEXT4AdoptsAnEstablishedVolumeWithoutFormatOrGrow(t *testing.T) {
	f := &ext4Fake{contents: diskfmt.Contents{FS: diskfmt.EXT4, Label: "APPDATA"}, markerPresent: true}

	device, err := blockmount.Run(storage(f.deps()), diskfmt.EXT4, "APPDATA", "/storage", false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if device != "/dev/mmcblk1" {
		t.Errorf("Run device = %q, want /dev/mmcblk1", device)
	}
	want := []string{"inspect", "mount", "check-marker"}
	if !reflect.DeepEqual(f.calls, want) {
		t.Errorf("call order = %v, want %v — adoption must not format or grow", f.calls, want)
	}
}

// TestRunEXT4CrashDebrisWithNoMarkerReformats: a device with ext4's own
// recognisable superblock and the app's label, but no establishment marker
// and an empty root, is exactly what an interrupted format leaves behind —
// self-heals with no consent needed, same as blank media.
func TestRunEXT4CrashDebrisWithNoMarkerReformats(t *testing.T) {
	f := &ext4Fake{contents: diskfmt.Contents{FS: diskfmt.EXT4, Label: "APPDATA"}}

	if _, err := blockmount.Run(storage(f.deps()), diskfmt.EXT4, "APPDATA", "/storage", false); err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := []string{"inspect", "mount", "check-marker", "check-root", "unmount", "format", "sync", "mount", "grow", "marker"}
	if !reflect.DeepEqual(f.calls, want) {
		t.Errorf("call order = %v, want %v", f.calls, want)
	}
}

// TestFSMismatchEstablishedFAT32PlusEXT4DefaultRefusesWithoutDestructive is
// the upgrade story this bean's fs-match adoption rule exists for: an eMMC
// an earlier build of the app formatted as FAT32, now asked for with the
// new zero-value (ext4) default, refuses rather than silently reformatting
// and destroying the FAT32 data — the error is actionable about the
// upgrade path (reformat destroys data; pass fat32 explicitly to keep
// adopting).
func TestFSMismatchEstablishedFAT32PlusEXT4DefaultRefusesWithoutDestructive(t *testing.T) {
	f := &ext4Fake{contents: diskfmt.Contents{FS: diskfmt.FAT32, Label: "APPDATA"}}

	_, err := blockmount.Run(storage(f.deps()), diskfmt.EXT4, "APPDATA", "/storage", false)
	if !errors.Is(err, ErrRefusedFormat) {
		t.Fatalf("Run error = %v, want ErrRefusedFormat", err)
	}
	for _, want := range []string{"FAT32", "ext4", "destructive=true"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Run error = %q, want it to mention %q (actionable upgrade story)", err, want)
		}
	}
	if len(f.calls) != 1 || f.calls[0] != "inspect" {
		t.Errorf("calls = %v, want only inspect — nothing touched before the caller decides", f.calls)
	}
}

// TestFSMismatchEstablishedFAT32PlusEXT4DefaultReformatsWhenDestructive is
// the other half: Destructive: true authorises the upgrade, reformatting the
// FAT32 eMMC as ext4 through the full establishment sequence.
func TestFSMismatchEstablishedFAT32PlusEXT4DefaultReformatsWhenDestructive(t *testing.T) {
	f := &ext4Fake{contents: diskfmt.Contents{FS: diskfmt.FAT32, Label: "APPDATA"}}

	if _, err := blockmount.Run(storage(f.deps()), diskfmt.EXT4, "APPDATA", "/storage", true); err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := []string{"inspect", "format", "sync", "mount", "grow", "marker"}
	if !reflect.DeepEqual(f.calls, want) {
		t.Errorf("call order = %v, want %v", f.calls, want)
	}
	if f.contents.FS != diskfmt.EXT4 {
		t.Errorf("formatted as %s, want ext4", f.contents.FS)
	}
}

// TestRunEXT4PreflightNamesBoardReality confirms the ext4-specific
// unsupported-kernel error reaches emmc's callers with the eMMC named,
// exactly as disk's does.
func TestRunEXT4PreflightNamesBoardReality(t *testing.T) {
	f := &ext4Fake{contents: diskfmt.Contents{Blank: true}, unmountable: diskfmt.EXT4}

	_, err := blockmount.Run(storage(f.deps()), diskfmt.EXT4, "APPDATA", "/storage", false)
	if !errors.Is(err, ErrUnsupportedFS) {
		t.Fatalf("Run error = %v, want ErrUnsupportedFS", err)
	}
	if !strings.Contains(err.Error(), "the eMMC at /dev/mmcblk1") {
		t.Errorf("Run error = %q, want it to name the eMMC and its device node", err)
	}
	if len(f.calls) != 1 || f.calls[0] != "inspect" {
		t.Errorf("calls = %v, want only inspect — the eMMC must be untouched", f.calls)
	}
}

// TestEXT4LabelCapIsSixteenBytes pins the ext4-specific label rule reaching
// emmc: a label FAT32/exFAT would reject at 11 bytes is fine for ext4 up to
// its own 16-byte width.
func TestEXT4LabelCapIsSixteenBytes(t *testing.T) {
	f := &ext4Fake{contents: diskfmt.Contents{Blank: true}}
	if _, err := blockmount.Run(storage(f.deps()), diskfmt.EXT4, "FOURTEEN_CHARS", "/storage", false); err != nil {
		t.Errorf("a 14-byte label was rejected for ext4: %v", err)
	}

	f = &ext4Fake{}
	_, err := blockmount.Run(storage(f.deps()), diskfmt.EXT4, "SEVENTEEN_CHARACT", "/storage", false)
	if err == nil {
		t.Fatal("Run accepted a 17-character ext4 label")
	}
	if !strings.Contains(err.Error(), "emmc: ") || !strings.Contains(err.Error(), "16") {
		t.Errorf("Run error = %q, want an emmc-prefixed complaint naming ext4's 16-byte limit", err)
	}
	if len(f.calls) != 0 {
		t.Errorf("calls = %v, want none — an invalid label must be caught before any device is touched", f.calls)
	}
}

// TestRunFAT32EstablishesThroughTheSharedSequence proves emmc's own wiring
// reaches the FAT32/exFAT half of the establishment machinery bean gosd-mm6q
// added — the device sync and the marker, which this package's deps supply —
// rather than the bare format-and-mount it used to get. The logic itself is
// tested package-agnostically in internal/blockmount.
func TestRunFAT32EstablishesThroughTheSharedSequence(t *testing.T) {
	f := &ext4Fake{contents: diskfmt.Contents{Blank: true}}

	if _, err := blockmount.Run(storage(f.deps()), diskfmt.FAT32, "APPDATA", "/storage", false); err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := []string{"inspect", "format", "sync", "mount", "marker"}
	if !reflect.DeepEqual(f.calls, want) {
		t.Errorf("call order = %v, want %v — no grow for FAT32, but a sync and a marker", f.calls, want)
	}
}

// TestRunFAT32AdoptsALegacyVolumeWithContent is the compatibility case seen
// from emmc: an eMMC formatted by an older release carries no marker, and
// must be adopted on the evidence of its own contents rather than reformatted
// out from under the app.
func TestRunFAT32AdoptsALegacyVolumeWithContent(t *testing.T) {
	f := &ext4Fake{contents: diskfmt.Contents{FS: diskfmt.FAT32, Label: "APPDATA"}, rootHasOther: true}

	if _, err := blockmount.Run(storage(f.deps()), diskfmt.FAT32, "APPDATA", "/storage", false); err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, call := range f.calls {
		if call == "format" {
			t.Fatalf("reformatted a legacy eMMC volume; calls = %v", f.calls)
		}
	}
}
