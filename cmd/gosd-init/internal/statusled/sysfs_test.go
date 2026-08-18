package statusled

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSysfsDiscoversLazilyAndCachesTheResult(t *testing.T) {
	// /sys isn't mounted until gosd-init's own early mounts run, which is
	// after main.go constructs a Sysfs but before Booting — the first call
	// — is ever made: New must not touch the filesystem itself.
	root := filepath.Join(t.TempDir(), "not-yet-mounted")
	s := New(root)

	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	makeGPIOLED(t, root, "ACT")

	if err := s.Booting(); err != nil {
		t.Fatalf("Booting() error = %v", err)
	}
	assertFileContent(t, filepath.Join(root, "ACT", "trigger"), "timer")

	// A second LED appearing after the first call must not change what was
	// already resolved: discovery only ever runs once.
	makeGPIOLED(t, root, "PWR")
	if err := s.Fatal(); err != nil {
		t.Fatalf("Fatal() error = %v", err)
	}
	assertFileContent(t, filepath.Join(root, "ACT", "trigger"), "none")
	if _, err := os.Stat(filepath.Join(root, "PWR", "trigger")); err == nil {
		t.Error("PWR was written to; discovery should have been cached to ACT before PWR ever appeared")
	}
}

func TestSysfsWithNoLEDFoundIsASilentNoOp(t *testing.T) {
	s := New(t.TempDir())

	if err := s.Booting(); err != nil {
		t.Errorf("Booting() error = %v, want nil", err)
	}
	if err := s.Running(); err != nil {
		t.Errorf("Running() error = %v, want nil", err)
	}
	if err := s.Fatal(); err != nil {
		t.Errorf("Fatal() error = %v, want nil", err)
	}
}
