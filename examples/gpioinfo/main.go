//go:build linux

// Command gpioinfo is a minimal example demonstrating GPIO access on GoSD:
// by default it opens every /dev/gpiochipN character device present and
// prints a gpioinfo(1)-style dump of each chip's name, label, line count,
// and per-line name/consumer/direction — entirely read-only, so it's safe
// to run on any board regardless of what's wired to its header.
//
// Setting both GOSD_GPIO_CHIP and GOSD_GPIO_LINE opts into a second,
// destructive step: that one line is requested as an output and toggled a
// few times, logging each transition. This never happens unless both env
// vars are set, since driving an arbitrary line on unknown wiring can
// short two outputs together or upset whatever's already connected.
//
// It talks to the kernel via the modern /dev/gpiochip character-device API
// through github.com/warthog618/go-gpiocdev, rather than the deprecated
// sysfs GPIO interface. For real applications, start from periph.io
// instead — see docs/runtime.md's "GPIO, I2C, SPI" section.
package main

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/warthog618/go-gpiocdev"
)

// blinkToggles is how many times the opt-in blink step flips the requested
// line, and blinkInterval is the pause between each flip - enough
// transitions to be obviously a blink on a multimeter or LED, brief enough
// that the example doesn't hang around waiting.
const (
	blinkToggles  = 6
	blinkInterval = 500 * time.Millisecond
)

func main() {
	chips := gpiocdev.Chips()
	if len(chips) == 0 {
		fmt.Println("gosd gpioinfo: no GPIO character devices found - is this board's GPIO controller enabled? see docs/runtime.md")
		return
	}

	for _, chip := range chips {
		describeChip(chip)
	}

	chipEnv, hasChip := os.LookupEnv("GOSD_GPIO_CHIP")
	lineEnv, hasLine := os.LookupEnv("GOSD_GPIO_LINE")
	if !hasChip && !hasLine {
		return
	}
	if hasChip != hasLine {
		fmt.Fprintln(os.Stderr, "gosd gpioinfo: GOSD_GPIO_CHIP and GOSD_GPIO_LINE must both be set to opt into the blink step; only one was")
		os.Exit(1)
	}

	offset, err := strconv.Atoi(lineEnv)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gosd gpioinfo: GOSD_GPIO_LINE %q isn't a line offset: %v\n", lineEnv, err)
		os.Exit(1)
	}

	if err := blink(chipEnv, offset); err != nil {
		fmt.Fprintf(os.Stderr, "gosd gpioinfo: %v\n", err)
		os.Exit(1)
	}
}

// describeChip prints one chip's identity and a gpioinfo(1)-style dump of
// every line it exposes. Failure to open a chip is reported and skipped
// rather than aborting the whole enumeration - a chip could in principle
// disappear or be permission-denied between listing and opening it.
func describeChip(chip string) {
	c, err := gpiocdev.NewChip(chip)
	if err != nil {
		fmt.Printf("%s: opening failed: %v\n", chip, err)
		return
	}
	defer func() { _ = c.Close() }()

	fmt.Printf("%s - %s (%d lines):\n", c.Name, c.Label, c.Lines())
	for offset := 0; offset < c.Lines(); offset++ {
		info, err := c.LineInfo(offset)
		if err != nil {
			fmt.Printf("  line %3d: reading info failed: %v\n", offset, err)
			continue
		}
		printLine(info)
	}
}

func printLine(info gpiocdev.LineInfo) {
	name := info.Name
	if name == "" {
		name = "unnamed"
	}
	usage := "unused"
	if info.Used {
		consumer := info.Consumer
		if consumer == "" {
			consumer = "unknown"
		}
		usage = fmt.Sprintf("used by %q", consumer)
	}
	fmt.Printf("  line %3d: %-20q %-10s %s\n", info.Offset, name, direction(info.Config.Direction), usage)
}

func direction(d gpiocdev.LineDirection) string {
	switch d {
	case gpiocdev.LineDirectionInput:
		return "input"
	case gpiocdev.LineDirectionOutput:
		return "output"
	default:
		return "unknown"
	}
}

// blink requests offset on chip as an output and toggles it a fixed number
// of times, logging each step, then reverts the line to an input before
// releasing it - the same "leave it as we found it" courtesy as the
// toggle_line_value example in go-gpiocdev itself.
func blink(chip string, offset int) error {
	fmt.Printf("gosd gpioinfo: requesting %s:%d as output for a %d-step blink\n", chip, offset, blinkToggles)

	l, err := gpiocdev.RequestLine(chip, offset, gpiocdev.AsOutput(0))
	if err != nil {
		return fmt.Errorf("requesting %s:%d as output: %w", chip, offset, err)
	}
	defer func() {
		_ = l.Reconfigure(gpiocdev.AsInput)
		_ = l.Close()
	}()

	value := 0
	for i := 0; i < blinkToggles; i++ {
		value ^= 1
		if err := l.SetValue(value); err != nil {
			return fmt.Errorf("setting %s:%d: %w", chip, offset, err)
		}
		state := "low"
		if value == 1 {
			state = "high"
		}
		fmt.Printf("gosd gpioinfo: %s:%d -> %s\n", chip, offset, state)
		time.Sleep(blinkInterval)
	}

	fmt.Printf("gosd gpioinfo: %s:%d reverted to input\n", chip, offset)
	return nil
}
