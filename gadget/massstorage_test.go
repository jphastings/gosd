package gadget

import (
	"strings"
	"testing"
)

const msLUN = gadgetRoot + "/functions/mass_storage.usb0/lun.0"

func TestMassStorageWritesLUNAttributes(t *testing.T) {
	f := newFakeFS()
	seedUDC(f, "20980000.usb")
	g := testGadget(MassStorage{Path: "/dev/nvme0n1p1", ReadOnly: true, Removable: true})

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
	g := testGadget(MassStorage{Path: "/dev/mmcblk0p3"})

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
	g := testGadget(MassStorage{Path: "/dev/nvme0n1p1", ReadOnly: true})

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
	g := testGadget(MassStorage{Path: "/dev/nvme0n1p1"})
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
