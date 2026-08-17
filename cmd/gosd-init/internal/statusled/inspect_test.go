package statusled

import (
	"path/filepath"
	"strings"
	"testing"
)

// The three shapes a bench run has to tell apart from the console line
// alone, since gosd-init offers no other way to ask (bean gosd-ddz6).

func TestDescribeNamesTheSelectedLEDAndWhatItChoseFrom(t *testing.T) {
	root := t.TempDir()
	makeGPIOLED(t, root, "green:status")
	makeGPIOLED(t, root, "red:heartbeat")

	got := New(root).Describe()

	if !strings.Contains(got, "using green:status") {
		t.Errorf("Describe() = %q, want it to name green:status as selected", got)
	}
	if !strings.Contains(got, "red:heartbeat") {
		t.Errorf("Describe() = %q, want it to list the candidate it passed over", got)
	}
}

func TestDescribeNamesRejectedEntriesWhenNothingQualifies(t *testing.T) {
	root := t.TempDir()
	makeNonGPIOLED(t, root, "input0::capslock")

	got := New(root).Describe()

	if !strings.Contains(got, "input0::capslock") {
		t.Errorf("Describe() = %q, want the rejected entry named: that is what distinguishes a filter bug from an empty class dir", got)
	}
}

func TestDescribeReportsNoLEDsAtAllDistinctly(t *testing.T) {
	got := New(filepath.Join(t.TempDir(), "does-not-exist")).Describe()

	if !strings.Contains(got, "no LEDs registered") {
		t.Errorf("Describe() = %q, want it to say no LEDs were registered", got)
	}
}

func TestDescribeAgreesWithTheLEDTheStatesDrive(t *testing.T) {
	root := t.TempDir()
	makeGPIOLED(t, root, "blue:activity")
	makeGPIOLED(t, root, "green:power")

	s := New(root)
	led, found, err := Discover(root)
	if err != nil || !found {
		t.Fatalf("Discover() = %v, %v, %v", led.Name(), found, err)
	}

	if !strings.Contains(s.Describe(), "using "+led.Name()) {
		t.Errorf("Describe() = %q, want it to name the same LED Discover selected (%q)", s.Describe(), led.Name())
	}
}

func TestInspectSortsItsListsSoTheLoggedLineIsStable(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"green:status", "ACT", "red:heartbeat"} {
		makeGPIOLED(t, root, name)
	}

	d, err := Inspect(root)
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}

	want := []string{"ACT", "green:status", "red:heartbeat"}
	if len(d.Candidates) != len(want) {
		t.Fatalf("Candidates = %v, want %v", d.Candidates, want)
	}
	for i, w := range want {
		if d.Candidates[i] != w {
			t.Fatalf("Candidates = %v, want %v (sorted, since os.ReadDir order isn't specified)", d.Candidates, want)
		}
	}
}
