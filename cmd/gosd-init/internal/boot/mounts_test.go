package boot

import (
	"strings"
	"testing"
	"time"
)

func TestMountEarlyMountsEverythingInOrder(t *testing.T) {
	m := &fakeMounter{}

	if err := mountEarly(m); err != nil {
		t.Fatalf("mountEarly() = %v, want nil", err)
	}

	wantTargets := []string{"/dev", "/proc", "/sys", "/run"}
	if len(m.calls) != len(wantTargets) {
		t.Fatalf("mountEarly() made %d Mount calls, want %d", len(m.calls), len(wantTargets))
	}
	for i, target := range wantTargets {
		if m.calls[i].target != target {
			t.Errorf("Mount call %d target = %q, want %q", i, m.calls[i].target, target)
		}
	}
}

func TestMountEarlyStopsAtFirstFailure(t *testing.T) {
	m := &fakeMounter{fn: func(c mountCall) error {
		if c.target == "/proc" {
			return errBoom
		}
		return nil
	}}

	err := mountEarly(m)
	if err == nil {
		t.Fatal("mountEarly() = nil, want error")
	}
	if got := m.callsFor("/sys"); got != 0 {
		t.Errorf("mountEarly() mounted /sys after /proc failed; should have stopped")
	}
}

func TestMountBootPartitionTriesEachDeviceInOrder(t *testing.T) {
	devices := []string{"/dev/mmcblk0p1", "/dev/mmcblk1p1"}
	m := &fakeMounter{fn: func(c mountCall) error {
		if c.source == "/dev/mmcblk1p1" {
			return nil
		}
		return errBoom
	}}
	clock := newFakeClock(time.Unix(0, 0))

	err := MountBootPartition(m, "/boot", devices, 10*time.Second, clock.Sleep, clock.Now)
	if err != nil {
		t.Fatalf("MountBootPartition() = %v, want nil", err)
	}
	if got := m.callsFor("/boot"); got != 2 {
		t.Fatalf("MountBootPartition() made %d attempts, want 2 (first device fails, second succeeds)", got)
	}
}

func TestMountBootPartitionRetriesUntilSuccess(t *testing.T) {
	devices := []string{"/dev/mmcblk0p1", "/dev/mmcblk1p1"}
	attempt := 0
	m := &fakeMounter{fn: func(c mountCall) error {
		attempt++
		// Fail the first two full rounds (4 attempts), succeed on the third round.
		if attempt <= 4 {
			return errBoom
		}
		return nil
	}}
	clock := newFakeClock(time.Unix(0, 0))

	err := MountBootPartition(m, "/boot", devices, 10*time.Second, clock.Sleep, clock.Now)
	if err != nil {
		t.Fatalf("MountBootPartition() = %v, want nil", err)
	}
	if attempt != 5 {
		t.Fatalf("MountBootPartition() made %d Mount attempts, want 5", attempt)
	}
}

func TestMountBootPartitionGivesUpAfterTimeout(t *testing.T) {
	devices := []string{"/dev/mmcblk0p1", "/dev/mmcblk1p1"}
	m := &fakeMounter{fn: func(mountCall) error { return errBoom }}
	clock := newFakeClock(time.Unix(0, 0))

	err := MountBootPartition(m, "/boot", devices, 10*time.Second, clock.Sleep, clock.Now)
	if err == nil {
		t.Fatal("MountBootPartition() = nil, want error after exhausting the timeout")
	}
	for _, dev := range devices {
		if !strings.Contains(err.Error(), dev) {
			t.Errorf("error %q does not mention tried device %q", err, dev)
		}
	}
	if clock.Now().Sub(time.Unix(0, 0)) < 10*time.Second {
		t.Errorf("MountBootPartition() gave up before the 10s timeout elapsed")
	}
}

var errBoom = mountError("boom")

type mountError string

func (e mountError) Error() string { return string(e) }
