package statusled

import (
	"fmt"
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

// Fatal must be STEADY, never a blink: gosd-init halts the board straight
// after setting it, and a halted kernel stops the timer trigger dead (proven
// on nanopi-zero2 - the old blink lasted ~100ms). Claiming the "none" trigger
// is what takes the LED off whatever the board shipped, so the brightness
// write sticks.
func TestFatalIsSteadyOnRatherThanBlinking(t *testing.T) {
	root := t.TempDir()
	makeGPIOLED(t, root, "ACT") // max_brightness = 255
	led := LED{root: root, name: "ACT"}

	if err := led.Fatal(); err != nil {
		t.Fatalf("Fatal() error = %v", err)
	}

	assertFileContent(t, filepath.Join(root, "ACT", "trigger"), "none")
	assertFileContent(t, filepath.Join(root, "ACT", "brightness"), "255")

	for _, blinkFile := range []string{"delay_on", "delay_off"} {
		if _, err := os.Stat(filepath.Join(root, "ACT", blinkFile)); err == nil {
			t.Errorf("Fatal() wrote %s: it must not blink, or the signal dies with the kernel", blinkFile)
		}
	}
}

// Running is a short blip against a mostly-dark LED, which is what keeps it
// distinguishable from Booting's even flash and from Fatal's steady level.
func TestRunningBlipsBrieflyOncePerSecond(t *testing.T) {
	root := t.TempDir()
	makeGPIOLED(t, root, "ACT")
	led := LED{root: root, name: "ACT"}

	if err := led.Running(); err != nil {
		t.Fatalf("Running() error = %v", err)
	}

	assertFileContent(t, filepath.Join(root, "ACT", "trigger"), "timer")
	assertFileContent(t, filepath.Join(root, "ACT", "delay_on"), "50")
	assertFileContent(t, filepath.Join(root, "ACT", "delay_off"), "950")
}

// The three states have to be told apart by eye, so no two may drive the
// LED the same way.
func TestTheThreeStatesAreVisiblyDistinct(t *testing.T) {
	read := func(state func(LED) error) map[string]string {
		root := t.TempDir()
		makeGPIOLED(t, root, "ACT")
		led := LED{root: root, name: "ACT"}
		if err := state(led); err != nil {
			t.Fatalf("state error = %v", err)
		}
		got := map[string]string{}
		for _, f := range []string{"trigger", "delay_on", "delay_off", "brightness"} {
			if b, err := os.ReadFile(filepath.Join(root, "ACT", f)); err == nil {
				got[f] = string(b)
			}
		}
		return got
	}

	states := map[string]map[string]string{
		"booting": read(LED.Booting),
		"running": read(LED.Running),
		"fatal":   read(LED.Fatal),
	}
	for a, av := range states {
		for b, bv := range states {
			if a < b && fmt.Sprint(av) == fmt.Sprint(bv) {
				t.Errorf("%s and %s drive the LED identically (%v)", a, b, av)
			}
		}
	}
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

// Fatal claims the "none" trigger before writing brightness: every board
// ships some default trigger, and a brightness written first is simply
// overwritten by whatever trigger still owns the LED.
func TestFatalWritesTriggerBeforeBrightness(t *testing.T) {
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
	if err := led.Fatal(); err != nil {
		t.Fatalf("Fatal() error = %v", err)
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
