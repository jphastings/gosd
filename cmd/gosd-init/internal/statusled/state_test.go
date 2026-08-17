package statusled

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestBootingClaimsTimerTriggerThenSetsQuarterSecondDelays(t *testing.T) {
	root := t.TempDir()
	makeGPIOLED(t, root, "ACT")
	led := LED{root: root, name: "ACT"}

	if err := led.Booting(); err != nil {
		t.Fatalf("Booting() error = %v", err)
	}

	assertFileContent(t, filepath.Join(root, "ACT", "trigger"), "timer")
	assertFileContent(t, filepath.Join(root, "ACT", "delay_on"), "250")
	assertFileContent(t, filepath.Join(root, "ACT", "delay_off"), "250")
}

func TestFatalBlinksTwiceAsFastAsBooting(t *testing.T) {
	root := t.TempDir()
	makeGPIOLED(t, root, "ACT")
	led := LED{root: root, name: "ACT"}

	if err := led.Fatal(); err != nil {
		t.Fatalf("Fatal() error = %v", err)
	}

	assertFileContent(t, filepath.Join(root, "ACT", "trigger"), "timer")
	assertFileContent(t, filepath.Join(root, "ACT", "delay_on"), "125")
	assertFileContent(t, filepath.Join(root, "ACT", "delay_off"), "125")
}

func TestRunningClaimsNoneTriggerThenSetsMaxBrightness(t *testing.T) {
	root := t.TempDir()
	makeGPIOLED(t, root, "ACT") // max_brightness = 255
	led := LED{root: root, name: "ACT"}

	if err := led.Running(); err != nil {
		t.Fatalf("Running() error = %v", err)
	}

	assertFileContent(t, filepath.Join(root, "ACT", "trigger"), "none")
	assertFileContent(t, filepath.Join(root, "ACT", "brightness"), "255")
}

// TestBootingWritesTriggerBeforeTheDelays pins the load-bearing write order:
// on a real kernel, delay_on/delay_off only exist once "timer" is the active
// trigger, so writing either first would fail. A plain temp-dir fake can't
// reproduce that failure (os.WriteFile happily creates any file in any
// order), so this observes the actual call order through the writeFile seam
// instead of inferring it from file contents.
func TestBootingWritesTriggerBeforeTheDelays(t *testing.T) {
	var calls []string
	old := writeFile
	writeFile = func(name string, _ []byte, _ os.FileMode) error {
		calls = append(calls, filepath.Base(name))
		return nil
	}
	defer func() { writeFile = old }()

	led := LED{root: "/fake", name: "ACT"}
	if err := led.Booting(); err != nil {
		t.Fatalf("Booting() error = %v", err)
	}

	want := []string{"trigger", "delay_on", "delay_off"}
	if !slices.Equal(calls, want) {
		t.Errorf("write order = %v, want %v", calls, want)
	}
}

func TestRunningWritesTriggerBeforeBrightness(t *testing.T) {
	var calls []string
	old := writeFile
	writeFile = func(name string, _ []byte, _ os.FileMode) error {
		calls = append(calls, filepath.Base(name))
		return nil
	}
	defer func() { writeFile = old }()

	root := t.TempDir()
	makeGPIOLED(t, root, "ACT")
	led := LED{root: root, name: "ACT"}
	if err := led.Running(); err != nil {
		t.Fatalf("Running() error = %v", err)
	}

	want := []string{"trigger", "brightness"}
	if !slices.Equal(calls, want) {
		t.Errorf("write order = %v, want %v", calls, want)
	}
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	if got := strings.TrimSpace(string(data)); got != want {
		t.Errorf("%s = %q, want %q", path, got, want)
	}
}
