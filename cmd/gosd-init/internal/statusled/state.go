package statusled

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// The three states' timings, in milliseconds (bean gosd-n82u).
//
// Booting and Running both blink, and are told apart by duty cycle rather
// than rate alone: booting is an even flash, running a short regular blip
// against a mostly-dark LED. Failure is the odd one out and is steady, not
// because a blink would be unclear but because it is impossible — see Fatal.
const (
	bootingOnMillis  = 250
	bootingOffMillis = 250
	runningOnMillis  = 50
	runningOffMillis = 950
)

// Booting sets an even 250ms flash: gosd-init is still starting up. The
// kernel's own timer trigger does this, so it carries on flashing even if
// gosd-init wedges — which is the whole point of it, and remains true
// however badly userspace fails, because a kernel timer does not care.
func (l LED) Booting() error {
	return l.blink(bootingOnMillis, bootingOffMillis)
}

// Running sets a short 50ms blip once a second: /app has started and been
// handed control. A blip rather than solid on, so that a healthy board
// visibly reads as alive rather than merely lit, and so the running state
// cannot be confused with Fatal's steady level.
func (l LED) Running() error {
	return l.blink(runningOnMillis, runningOffMillis)
}

// Fatal sets the LED steady on to mark a recorded fatal error.
//
// Steady rather than blinking because gosd-init halts the board immediately
// afterwards, and a halted kernel cannot blink: the timer trigger is a
// kernel timer, so it stops the moment the kernel stops scheduling. This was
// proven on nanopi-zero2 — the old fast blink existed for about 100ms before
// "reboot: System halted" and was invisible.
//
// A steady level survives only if the board's device tree carries
// retain-state-shutdown on this LED; without it, gpio_led_shutdown() turns
// the LED off during device_shutdown(). That DT work is bean gosd-54j8, and
// until it ships the LED goes dark at halt on every board — the same as
// before this change, so nothing here depends on it.
func (l LED) Fatal() error {
	if err := l.write("trigger", "none"); err != nil {
		return err
	}
	max, err := os.ReadFile(l.path("max_brightness"))
	if err != nil {
		return fmt.Errorf("reading max_brightness: %w", err)
	}
	return l.write("brightness", strings.TrimSpace(string(max)))
}

// blink claims the LED via the "timer" trigger and only then sets its on and
// off delays. That order is load-bearing, not stylistic: delay_on and
// delay_off only exist once "timer" is the active trigger, so writing either
// first fails against a real sysfs tree. Claiming the trigger is also what
// takes the LED off whatever default the board shipped (mmc0, actpwr,
// heartbeat, default-on).
func (l LED) blink(onMillis, offMillis int) error {
	if err := l.write("trigger", "timer"); err != nil {
		return err
	}
	if err := l.write("delay_on", strconv.Itoa(onMillis)); err != nil {
		return err
	}
	return l.write("delay_off", strconv.Itoa(offMillis))
}

func (l LED) path(file string) string {
	return filepath.Join(l.root, l.name, file)
}

// writeFile is os.WriteFile, indirected so tests can observe the exact
// order LED's methods write sysfs files in — the load-bearing detail this
// package's doc and blink's comment call out — without needing a real
// filesystem that itself enforces it.
var writeFile = os.WriteFile

func (l LED) write(file, value string) error {
	if err := writeFile(l.path(file), []byte(value), 0o644); err != nil {
		return fmt.Errorf("writing %q to %s: %w", value, file, err)
	}
	return nil
}
