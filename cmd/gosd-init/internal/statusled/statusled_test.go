package statusled

import (
	"os"
	"path/filepath"
	"testing"
)

// makeGPIOLED creates a fake sysfs LED entry under root whose
// device/of_node/compatible names gpio-leds, the candidate filter's
// positive proof.
func makeGPIOLED(t *testing.T, root, name string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Join(dir, "device", "of_node"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "device", "of_node", "compatible"), []byte("gpio-leds\x00"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "max_brightness"), []byte("255\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// makeNonGPIOLED creates a sysfs LED entry with no of_node/compatible file
// at all, mirroring input0::capslock: an input-class LED (CONFIG_INPUT_LEDS,
// on by default) has no gpio-leds parent to prove.
func makeNonGPIOLED(t *testing.T, root, name string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, name), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoverPi3bShapePicksACT(t *testing.T) {
	root := t.TempDir()
	makeGPIOLED(t, root, "ACT")
	makeGPIOLED(t, root, "PWR")

	led, found, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if !found {
		t.Fatal("Discover() found = false, want true")
	}
	if led.Name() != "ACT" {
		t.Errorf("Discover() selected %q, want ACT (label ACT wins over PWR)", led.Name())
	}
}

func TestDiscoverNanopiZero2ShapePicksGreenStatus(t *testing.T) {
	root := t.TempDir()
	makeGPIOLED(t, root, "red:heartbeat")
	makeGPIOLED(t, root, "green:status")

	led, found, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if !found || led.Name() != "green:status" {
		t.Errorf("Discover() = %q, %v, want green:status, true (heartbeat doesn't count as status)", led.Name(), found)
	}
}

func TestDiscoverCubieA5eShapePicksBlueActivity(t *testing.T) {
	root := t.TempDir()
	makeGPIOLED(t, root, "green:power")
	makeGPIOLED(t, root, "blue:activity")

	led, found, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if !found || led.Name() != "blue:activity" {
		t.Errorf("Discover() = %q, %v, want blue:activity, true (activity outranks green)", led.Name(), found)
	}
}

func TestDiscoverExcludesAnInputClassLED(t *testing.T) {
	root := t.TempDir()
	makeNonGPIOLED(t, root, "input0::capslock")
	makeGPIOLED(t, root, "ACT")

	led, found, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if !found || led.Name() != "ACT" {
		t.Errorf("Discover() = %q, %v, want ACT, true (input0::capslock must be excluded)", led.Name(), found)
	}
}

func TestDiscoverNoLEDsFoundIsNotAnError(t *testing.T) {
	root := t.TempDir()
	makeNonGPIOLED(t, root, "input0::capslock")

	_, found, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover() error = %v, want nil", err)
	}
	if found {
		t.Error("Discover() found = true, want false: the only entry isn't a gpio-leds candidate")
	}
}

func TestDiscoverMissingRootIsNotAnError(t *testing.T) {
	_, found, err := Discover(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("Discover() error = %v, want nil (a board with no LED sysfs class at all)", err)
	}
	if found {
		t.Error("Discover() found = true, want false")
	}
}

func TestParseName(t *testing.T) {
	cases := []struct {
		name                                string
		wantLabel, wantColour, wantFunction string
	}{
		{"ACT", "ACT", "", ""},
		{"PWR", "PWR", "", ""},
		{"green:heartbeat", "", "green", "heartbeat"},
		{"green:status", "", "green", "status"},
		{"blue:activity", "", "blue", "activity"},
		{"input1:red:mute", "", "red", "mute"},
	}
	for _, c := range cases {
		label, colour, function := parseName(c.name)
		if label != c.wantLabel || colour != c.wantColour || function != c.wantFunction {
			t.Errorf("parseName(%q) = (%q, %q, %q), want (%q, %q, %q)",
				c.name, label, colour, function, c.wantLabel, c.wantColour, c.wantFunction)
		}
	}
}

func TestSelectLEDIsStableRegardlessOfOrder(t *testing.T) {
	act := LED{name: "ACT", label: "ACT"}
	pwr := LED{name: "PWR", label: "PWR"}
	other := LED{name: "aux"}

	for _, candidates := range [][]LED{
		{act, pwr, other},
		{other, pwr, act},
		{pwr, act, other},
		{other, act, pwr},
	} {
		led, found := selectLED(candidates)
		if !found || led.Name() != "ACT" {
			t.Errorf("selectLED(%v) = %q, %v, want ACT, true", candidates, led.Name(), found)
		}
	}
}

func TestSelectLEDPrefersGreenWithinATier(t *testing.T) {
	// Both match tier 1 (function activity/status), so the tie is broken by
	// colour before it ever reaches the name comparison.
	blueStatus := LED{name: "blue:status", colour: "blue", function: "status"}
	greenActivity := LED{name: "green:activity", colour: "green", function: "activity"}

	led, found := selectLED([]LED{blueStatus, greenActivity})
	if !found || led.Name() != "green:activity" {
		t.Errorf("selectLED() = %q, %v, want green:activity (green preferred within the tier)", led.Name(), found)
	}
}

func TestSelectLEDBreaksRemainingTiesLexicographically(t *testing.T) {
	// Both match tier 1 (function status) and neither is green, so the tie
	// falls all the way through to the sysfs name.
	b := LED{name: "b:status", colour: "blue", function: "status"}
	a := LED{name: "a:status", colour: "red", function: "status"}

	led, found := selectLED([]LED{b, a})
	if !found || led.Name() != "a:status" {
		t.Errorf("selectLED() = %q, %v, want a:status (lexicographically smallest)", led.Name(), found)
	}
}
