// Package statusled discovers a board's onboard status LED through sysfs
// and drives it through the kernel's own "timer" trigger — never a
// goroutine — so the blink survives the very failures it's reporting on: a
// fault.Fatal halt, or gosd-init itself wedging mid-boot. See
// docs/status-led.md for the three states this drives and the resolved
// per-board table (bean gosd-xtcs).
package statusled

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DefaultRoot is where the kernel exposes every registered LED class
// device; main.go wires the real Sysfs implementation against it.
const DefaultRoot = "/sys/class/leds"

// LED is one status LED, already resolved to the sysfs entry Discover
// selected. Its zero value is not meaningful outside this package; callers
// only ever get one back from Discover.
type LED struct {
	root, name              string
	label, colour, function string
}

// Name is the LED's sysfs entry name, e.g. "ACT" or "green:status" — the
// same string a board's own device tree gives it.
func (l LED) Name() string { return l.name }

// gpioLEDsCompatible is the exact of_node/compatible entry that marks an
// LED class device as backed by the "gpio-leds" DT binding — the positive
// proof gosd-xtcs's candidate filter requires, rather than the mere absence
// of some other explanation. CONFIG_INPUT_LEDS=y on every board means a
// plugged-in USB keyboard's input0::capslock LED would otherwise pass every
// other check; requiring this parent is what excludes it, and any future
// PHY LED along with it.
const gpioLEDsCompatible = "gpio-leds"

// isGPIOLEDCandidate reports whether the LED class device name, under root,
// is backed by the gpio-leds DT binding: its device/of_node/compatible file
// must be readable and list gpio-leds among its NUL-separated strings.
func isGPIOLEDCandidate(root, name string) bool {
	data, err := os.ReadFile(filepath.Join(root, name, "device", "of_node", "compatible"))
	if err != nil {
		return false
	}
	for _, s := range strings.Split(string(data), "\x00") {
		if s == gpioLEDsCompatible {
			return true
		}
	}
	return false
}

// parseName splits a sysfs LED name on ":" per gosd-xtcs's locked rule: one
// part is a bare label (ACT, PWR); two are colour:function
// (green:heartbeat); three are devicename:colour:function. Any other shape
// (never seen on a real board) parses to nothing, which only ever costs it
// tier 4 ("anything remaining").
func parseName(name string) (label, colour, function string) {
	switch parts := strings.Split(name, ":"); len(parts) {
	case 1:
		return parts[0], "", ""
	case 2:
		return "", parts[0], parts[1]
	case 3:
		return "", parts[1], parts[2]
	default:
		return "", "", ""
	}
}

// Discover enumerates root (a sysfs LEDs class directory — DefaultRoot in
// production, a temp dir under test) and returns the board's status LED per
// gosd-xtcs's selection rule. found is false, with a nil error, whenever
// root doesn't exist or holds no gpio-leds candidate at all: a board with no
// onboard status LED is a normal outcome, not a failure (qemu-virt has
// none). A non-nil error means root exists but couldn't even be listed
// (permissions, a non-directory path) — still never worth failing or
// delaying boot over, but worth logging.
func Discover(root string) (LED, bool, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return LED{}, false, nil
		}
		return LED{}, false, err
	}

	var candidates []LED
	for _, entry := range entries {
		name := entry.Name()
		if !isGPIOLEDCandidate(root, name) {
			continue
		}
		label, colour, function := parseName(name)
		candidates = append(candidates, LED{root: root, name: name, label: label, colour: colour, function: function})
	}

	led, found := selectLED(candidates)
	return led, found, nil
}

// tier is one of gosd-xtcs's four selection tiers, tried in order; the
// first with any match at all wins outright, so a candidate lower down never
// gets to outrank one in an earlier tier no matter how the tie-break below
// would have ordered them.
type tier int

const (
	tierActivityStatus tier = iota
	tierGreen
	tierPower
	tierAny
)

// matchesTier reports whether l belongs to tier t. Deliberately, "heartbeat"
// never counts as tierActivityStatus (see the package doc's per-board
// table): it is what makes nanopi-zero2 pick its green:status LED over
// red:heartbeat.
func matchesTier(l LED, t tier) bool {
	switch t {
	case tierActivityStatus:
		return l.function == "activity" || l.function == "status" || strings.EqualFold(l.label, "ACT")
	case tierGreen:
		return l.colour == "green"
	case tierPower:
		return l.function == "power" || strings.EqualFold(l.label, "PWR") || strings.EqualFold(l.label, "POWER")
	default:
		return true
	}
}

// selectLED applies gosd-xtcs's tiered selection rule. candidate order must
// never decide the outcome — real sysfs directory order isn't specified —
// so ties are broken explicitly: first to the green one, then to the
// lexicographically smallest sysfs name.
func selectLED(candidates []LED) (LED, bool) {
	for t := tierActivityStatus; t <= tierAny; t++ {
		var matched []LED
		for _, c := range candidates {
			if matchesTier(c, t) {
				matched = append(matched, c)
			}
		}
		if len(matched) == 0 {
			continue
		}
		sort.Slice(matched, func(i, j int) bool {
			gi, gj := matched[i].colour == "green", matched[j].colour == "green"
			if gi != gj {
				return gi
			}
			return matched[i].name < matched[j].name
		})
		return matched[0], true
	}
	return LED{}, false
}
