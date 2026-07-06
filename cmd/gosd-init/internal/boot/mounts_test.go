package boot

import (
	"errors"
	"io/fs"
	"strings"
	"testing"
	"time"
)

func TestMountEarlyMountsEverythingInOrder(t *testing.T) {
	m := &fakeMounter{}

	if err := mountEarly(m); err != nil {
		t.Fatalf("mountEarly() = %v, want nil", err)
	}

	wantTargets := []string{"/dev", "/proc", "/sys", "/sys/kernel/config", "/run"}
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

func TestMountDataPartitionMountsReadWriteWithFlush(t *testing.T) {
	m := &fakeMounter{}
	clock := newFakeClock(time.Unix(0, 0))

	err := MountDataPartition(m, "/data", []string{"/dev/mmcblk0p2"}, 10*time.Second, clock.Sleep, clock.Now)
	if err != nil {
		t.Fatalf("MountDataPartition() = %v, want nil", err)
	}

	call := m.calls[0]
	if call.flags&msRdOnly != 0 {
		t.Error("data partition was mounted read-only; want read-write")
	}
	if call.data != "flush" {
		t.Errorf("data partition mount options = %q, want \"flush\"", call.data)
	}
	if call.fstype != "vfat" {
		t.Errorf("data partition fstype = %q, want vfat", call.fstype)
	}
}

func TestMountDataPartitionRetriesTransientFailures(t *testing.T) {
	attempt := 0
	m := &fakeMounter{fn: func(mountCall) error {
		attempt++
		if attempt <= 2 {
			return errBoom // transient, not ENOENT
		}
		return nil
	}}
	clock := newFakeClock(time.Unix(0, 0))

	err := MountDataPartition(m, "/data", []string{"/dev/mmcblk0p2", "/dev/mmcblk1p2"}, 10*time.Second, clock.Sleep, clock.Now)
	if err != nil {
		t.Fatalf("MountDataPartition() = %v, want nil after retrying transient failures", err)
	}
	if attempt != 3 {
		t.Errorf("MountDataPartition() made %d Mount attempts, want 3", attempt)
	}
}

func TestMountDataPartitionReportsMissingPartitionImmediately(t *testing.T) {
	// Both candidate device nodes not existing means the image has no data
	// partition; that must be detected on the first round rather than
	// burning the whole timeout on retries.
	m := &fakeMounter{fn: func(mountCall) error { return fs.ErrNotExist }}
	clock := newFakeClock(time.Unix(0, 0))

	err := MountDataPartition(m, "/data", []string{"/dev/mmcblk0p2", "/dev/mmcblk1p2"}, 10*time.Second, clock.Sleep, clock.Now)
	if !errors.Is(err, ErrDataPartitionMissing) {
		t.Fatalf("MountDataPartition() = %v, want ErrDataPartitionMissing", err)
	}
	if got := clock.Now().Sub(time.Unix(0, 0)); got != 0 {
		t.Errorf("MountDataPartition() slept %s before reporting a missing partition; want no delay", got)
	}
}

// TestMountDataPartitionReportsMissingPartitionImmediatelyWithThreeCandidates
// covers the longer, three-device candidate list gosd-init now probes
// (mmcblk0, mmcblk1, vda - see main.go): the fast-ENOENT path must still
// fire on the first round no matter how many candidates it has to check.
func TestMountDataPartitionReportsMissingPartitionImmediatelyWithThreeCandidates(t *testing.T) {
	m := &fakeMounter{fn: func(mountCall) error { return fs.ErrNotExist }}
	clock := newFakeClock(time.Unix(0, 0))

	devices := []string{"/dev/mmcblk0p2", "/dev/mmcblk1p2", "/dev/vda2"}
	err := MountDataPartition(m, "/data", devices, 10*time.Second, clock.Sleep, clock.Now)
	if !errors.Is(err, ErrDataPartitionMissing) {
		t.Fatalf("MountDataPartition() = %v, want ErrDataPartitionMissing", err)
	}
	if got := clock.Now().Sub(time.Unix(0, 0)); got != 0 {
		t.Errorf("MountDataPartition() slept %s before reporting a missing partition; want no delay", got)
	}
	if got := m.callsFor("/data"); got != len(devices) {
		t.Errorf("MountDataPartition() made %d attempts, want exactly %d (one per candidate, no retry round)", got, len(devices))
	}
}

func TestMountDataPartitionGivesUpAfterTimeout(t *testing.T) {
	m := &fakeMounter{fn: func(mountCall) error { return errBoom }}
	clock := newFakeClock(time.Unix(0, 0))

	err := MountDataPartition(m, "/data", []string{"/dev/mmcblk0p2"}, 10*time.Second, clock.Sleep, clock.Now)
	if err == nil {
		t.Fatal("MountDataPartition() = nil, want error after exhausting the timeout")
	}
	if errors.Is(err, ErrDataPartitionMissing) {
		t.Errorf("MountDataPartition() = %v; a persistent non-ENOENT failure must not read as a missing partition", err)
	}
	if clock.Now().Sub(time.Unix(0, 0)) < 10*time.Second {
		t.Error("MountDataPartition() gave up before the timeout elapsed")
	}
}

var errBoom = mountError("boom")

type mountError string

func (e mountError) Error() string { return string(e) }
