package main

import "testing"

// TestBootDevicesIncludesVirtioAfterMMC asserts the qemu-virt candidate
// (/dev/vda1) is present, and listed after the real mmcblk devices: the
// probe logic tries candidates in order, and mmcblk hardware is what real
// devices actually have, so it should be checked first.
func TestBootDevicesIncludesVirtioAfterMMC(t *testing.T) {
	want := []string{"/dev/mmcblk0p1", "/dev/mmcblk1p1", "/dev/vda1"}
	if !equal(bootDevices, want) {
		t.Errorf("bootDevices = %v, want %v", bootDevices, want)
	}
}

// TestDataDevicesIncludesVirtioAfterMMC is TestBootDevicesIncludesVirtioAfterMMC's
// counterpart for the optional GOSD-DATA partition's candidate list.
func TestDataDevicesIncludesVirtioAfterMMC(t *testing.T) {
	want := []string{"/dev/mmcblk0p2", "/dev/mmcblk1p2", "/dev/vda2"}
	if !equal(dataDevices, want) {
		t.Errorf("dataDevices = %v, want %v", dataDevices, want)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
