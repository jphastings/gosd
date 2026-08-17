package statusled

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// bootingDelayMillis and fatalDelayMillis are gosd-xtcs's locked timer-
// trigger delays, in milliseconds: 250ms on/off while booting, half that
// (125ms) once a fatal error has been recorded.
const (
	bootingDelayMillis = 250
	fatalDelayMillis   = 125
)

// Booting sets the LED blinking at 250ms on/off: gosd-init is still
// starting up.
func (l LED) Booting() error {
	return l.blink(bootingDelayMillis)
}

// Fatal sets the LED blinking at 125ms on/off — twice Booting's rate — to
// mark a recorded fatal error. Like Booting, this claims the kernel's
// "timer" trigger rather than blinking from a goroutine, which is what lets
// it keep blinking through the halt that follows it (see the package doc).
func (l LED) Fatal() error {
	return l.blink(fatalDelayMillis)
}

// blink claims the LED via the "timer" trigger and only then sets its
// on/off delay. That order is load-bearing, not stylistic: delay_on and
// delay_off only exist once "timer" is the active trigger, so writing
// either first would fail against a real sysfs tree.
func (l LED) blink(delayMillis int) error {
	if err := l.write("trigger", "timer"); err != nil {
		return err
	}
	delay := strconv.Itoa(delayMillis)
	if err := l.write("delay_on", delay); err != nil {
		return err
	}
	return l.write("delay_off", delay)
}

// Running sets the LED solid on: /app has started and been handed control.
// Every board ships some default trigger (mmc0, actpwr, heartbeat,
// default-on), so claiming "none" first is what makes the brightness write
// stick rather than being overwritten by whatever trigger already owned the
// LED.
func (l LED) Running() error {
	if err := l.write("trigger", "none"); err != nil {
		return err
	}
	max, err := os.ReadFile(l.path("max_brightness"))
	if err != nil {
		return fmt.Errorf("reading max_brightness: %w", err)
	}
	return l.write("brightness", strings.TrimSpace(string(max)))
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
