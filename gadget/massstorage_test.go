package gadget

import (
	"errors"
	"strings"
	"testing"
)

const msLUN = gadgetRoot + "/functions/mass_storage.usb0/lun.0"

// noneMounted is a mountedTargets fake reporting nothing mounted — the
// default for tests that aren't specifically exercising the mounted-device
// rejection below.
func noneMounted() (map[string]string, error) { return map[string]string{}, nil }

// mountedFake returns a mountedTargets fake reporting exactly one mounted
// device.
func mountedFake(source, target string) func() (map[string]string, error) {
	return func() (map[string]string, error) {
		return map[string]string{source: target}, nil
	}
}

func TestMassStorageWritesLUNAttributes(t *testing.T) {
	f := newFakeFS()
	seedUDC(f, "20980000.usb")
	g := testGadget(MassStorage{Path: "/dev/nvme0n1p1", ReadOnly: true, Removable: true, mountedTargets: noneMounted})

	if err := applyWithFake(t, g, f); err != nil {
		t.Fatalf("Apply() = %v, want nil", err)
	}

	want := map[string]string{
		msLUN + "/file":      "/dev/nvme0n1p1\n",
		msLUN + "/ro":        "1\n",
		msLUN + "/removable": "1\n",
	}
	for path, want := range want {
		got, ok := f.files[path]
		if !ok {
			t.Errorf("attribute %s was never written", path)
			continue
		}
		if string(got) != want {
			t.Errorf("attribute %s = %q, want %q", path, got, want)
		}
	}
	if _, ok := f.links[gadgetRoot+"/configs/c.1/mass_storage.usb0"]; !ok {
		t.Error("mass_storage.usb0 was not linked into config c.1")
	}
}

func TestMassStorageFlagsDefaultOff(t *testing.T) {
	f := newFakeFS()
	seedUDC(f, "20980000.usb")
	g := testGadget(MassStorage{Path: "/dev/mmcblk0p3", mountedTargets: noneMounted})

	if err := applyWithFake(t, g, f); err != nil {
		t.Fatalf("Apply() = %v, want nil", err)
	}

	for _, attr := range []string{"ro", "removable"} {
		if got := string(f.files[msLUN+"/"+attr]); got != "0\n" {
			t.Errorf("attribute %s = %q, want %q", attr, got, "0\n")
		}
	}
}

// The kernel refuses to change a LUN's flags once its backing file is open,
// so the write order (flags before file) is load-bearing kernel semantics,
// not an implementation detail.
func TestMassStorageWritesFlagsBeforeBackingFile(t *testing.T) {
	f := newFakeFS()
	seedUDC(f, "20980000.usb")
	g := testGadget(MassStorage{Path: "/dev/nvme0n1p1", ReadOnly: true, mountedTargets: noneMounted})

	if err := applyWithFake(t, g, f); err != nil {
		t.Fatalf("Apply() = %v, want nil", err)
	}

	fileIdx := f.indexOfCall("write", msLUN+"/file")
	for _, attr := range []string{"ro", "removable"} {
		idx := f.indexOfCall("write", msLUN+"/"+attr)
		if idx == -1 || fileIdx == -1 || idx > fileIdx {
			t.Errorf("attribute %s written at index %d, want before file at index %d", attr, idx, fileIdx)
		}
	}
}

func TestMassStorageEmptyPathFails(t *testing.T) {
	f := newFakeFS()
	seedUDC(f, "20980000.usb")
	g := testGadget(MassStorage{})

	err := applyWithFake(t, g, f)
	if err == nil {
		t.Fatal("Apply() = nil, want error for empty MassStorage.Path")
	}
	if !strings.Contains(err.Error(), "Path") {
		t.Errorf("error %q should name the missing Path field", err)
	}
}

// lun.0 is a kernel-owned configfs default group, removed with the function
// directory rather than individually — Close must tear the gadget down
// cleanly without tripping over it.
func TestCloseRemovesMassStorageFunction(t *testing.T) {
	f := newFakeFS()
	seedUDC(f, "20980000.usb")
	g := testGadget(MassStorage{Path: "/dev/nvme0n1p1", mountedTargets: noneMounted})
	if err := applyWithFake(t, g, f); err != nil {
		t.Fatalf("Apply() = %v, want nil", err)
	}

	if err := g.Close(); err != nil {
		t.Fatalf("Close() = %v, want nil", err)
	}

	for _, m := range []map[string]bool{f.dirs, f.defaultGroups} {
		for path := range m {
			if strings.HasPrefix(path, gadgetRoot) {
				t.Errorf("directory %s still exists after Close()", path)
			}
		}
	}
}

// TestMassStorageRejectsMountedPath covers the three containment directions
// gosd-fnh8 calls out: Path itself mounted, Path a partition of a mounted
// whole device, and Path the whole device of a mounted partition — across
// both partition-naming conventions (sd's bare digit, nvme's "p" digit).
func TestMassStorageRejectsMountedPath(t *testing.T) {
	tests := []struct {
		name             string
		path             string
		mountedSrc, atMP string
	}{
		{"path itself is mounted", "/dev/sda1", "/dev/sda1", "/mnt/usb"},
		{"path is a partition of a mounted whole device", "/dev/sda1", "/dev/sda", "/mnt/whole"},
		{"path is the parent device of a mounted partition", "/dev/sda", "/dev/sda1", "/mnt/usb"},
		{"nvme: path is the parent device of a mounted partition", "/dev/nvme0n1", "/dev/nvme0n1p1", "/storage"},
		{"nvme: path is a partition of a mounted whole device", "/dev/nvme0n1p1", "/dev/nvme0n1", "/storage"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeFS()
			seedUDC(f, "20980000.usb")
			g := testGadget(MassStorage{Path: tt.path, mountedTargets: mountedFake(tt.mountedSrc, tt.atMP)})

			err := applyWithFake(t, g, f)
			if err == nil {
				t.Fatalf("Apply() = nil, want error: %s is mounted at %s", tt.mountedSrc, tt.atMP)
			}
			if !strings.Contains(err.Error(), tt.atMP) {
				t.Errorf("error %q should name the mountpoint %q", err, tt.atMP)
			}
			if !strings.Contains(err.Error(), "Unmount") {
				t.Errorf("error %q should tell the caller to Unmount", err)
			}
			assertNoGadgetState(t, f)
		})
	}
}

// An unrelated device being mounted elsewhere must never block Path — only
// Path's own device (or a partition/parent of it) does.
func TestMassStorageAllowsUnrelatedMount(t *testing.T) {
	f := newFakeFS()
	seedUDC(f, "20980000.usb")
	g := testGadget(MassStorage{Path: "/dev/sda1", mountedTargets: mountedFake("/dev/sdb1", "/mnt/other")})

	if err := applyWithFake(t, g, f); err != nil {
		t.Fatalf("Apply() = %v, want nil: an unrelated device being mounted must not block Path", err)
	}
}

// TestMassStorageMountThenExposeWithoutUnmountFails pins the exact failure
// gosd-fnh8 exists to catch: the disk/emmc docs' own two snippets
// concatenated — FormatAndMount returns a still-mounted BlockDevice, and
// handing it straight to MassStorage without an intervening Unmount would
// corrupt the volume (the board's vfat cache and the USB host both writing
// raw blocks). Create must refuse loudly instead, and Apply must unwind
// cleanly (gosd-0r40's machinery), leaving no gadget state behind.
func TestMassStorageMountThenExposeWithoutUnmountFails(t *testing.T) {
	f := newFakeFS()
	seedUDC(f, "20980000.usb")
	device, mountpoint := "/dev/mmcblk0", "/storage"
	g := testGadget(MassStorage{Path: device, mountedTargets: mountedFake(device, mountpoint)})

	err := applyWithFake(t, g, f)
	if err == nil {
		t.Fatal("Apply() = nil, want error: Path is still mounted, matching the docs' forgotten-Unmount scenario")
	}
	if !strings.Contains(err.Error(), mountpoint) {
		t.Errorf("error %q should name the mountpoint %q", err, mountpoint)
	}
	assertNoGadgetState(t, f)
}

// TestMassStorageMountUnmountThenExposeSucceeds is the same scenario's
// happy path: once Unmount has actually run (the device no longer appears
// in mountedTargets), Apply succeeds and writes the LUN as normal.
func TestMassStorageMountUnmountThenExposeSucceeds(t *testing.T) {
	f := newFakeFS()
	seedUDC(f, "20980000.usb")
	device := "/dev/mmcblk0"
	g := testGadget(MassStorage{Path: device, mountedTargets: noneMounted})

	if err := applyWithFake(t, g, f); err != nil {
		t.Fatalf("Apply() after Unmount = %v, want nil", err)
	}
	if got := string(f.files[msLUN+"/file"]); got != device+"\n" {
		t.Errorf("lun.0/file = %q, want %q", got, device+"\n")
	}
}

// A failure reading the mounted-device state (e.g. /proc/mounts unreadable)
// must surface actionably rather than being swallowed, and must unwind
// cleanly like any other Create failure.
func TestMassStorageMountCheckErrorPropagates(t *testing.T) {
	f := newFakeFS()
	seedUDC(f, "20980000.usb")
	wantErr := errors.New("boom: /proc/mounts unreadable")
	g := testGadget(MassStorage{Path: "/dev/sda1", mountedTargets: func() (map[string]string, error) {
		return nil, wantErr
	}})

	err := applyWithFake(t, g, f)
	if err == nil || !strings.Contains(err.Error(), wantErr.Error()) {
		t.Fatalf("Apply() = %v, want error wrapping %v", err, wantErr)
	}
	assertNoGadgetState(t, f)
}

// TestRelatedDevicePaths exercises the naming heuristic behind the mounted-
// device rejection directly, across both partition-naming conventions and
// the cases that must NOT match: unrelated devices, an eMMC hardware
// partition (not a numbered one), and non-/dev file paths (a disk-image
// file backing a LUN follows no device-partition convention).
func TestRelatedDevicePaths(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want bool
	}{
		{"identical device", "/dev/sda1", "/dev/sda1", true},
		{"identical file", "/data/image.bin", "/data/image.bin", true},
		{"sd: partition of whole device", "/dev/sda1", "/dev/sda", true},
		{"sd: whole device of a partition", "/dev/sda", "/dev/sda1", true},
		{"nvme: partition of whole device", "/dev/nvme0n1p1", "/dev/nvme0n1", true},
		{"nvme: whole device of a partition", "/dev/nvme0n1", "/dev/nvme0n1p1", true},
		{"mmc: partition of whole device", "/dev/mmcblk0p1", "/dev/mmcblk0", true},
		{"mmc: whole device of a partition", "/dev/mmcblk0", "/dev/mmcblk0p1", true},
		{"unrelated sd devices", "/dev/sda1", "/dev/sdb1", false},
		{"unrelated nvme namespaces", "/dev/nvme0n1p1", "/dev/nvme0n2p1", false},
		{"unrelated mmc devices", "/dev/mmcblk0p1", "/dev/mmcblk1p1", false},
		{"eMMC hardware partition is not a numbered partition", "/dev/mmcblk0", "/dev/mmcblk0boot0", false},
		{"non-device file paths never partition-match", "/data/image.bin", "/data/image.bin2", false},
		{"pseudo-filesystem source never matches a device path", "/dev/sda1", "tmpfs", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := relatedDevicePaths(tt.a, tt.b); got != tt.want {
				t.Errorf("relatedDevicePaths(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
