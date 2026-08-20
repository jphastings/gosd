// Command usbserial is a minimal example app used to exercise GoSD's USB
// gadget mode end to end: it presents the board as a USB CDC-ACM serial
// device and echoes back every line it receives over /dev/ttyGS0. Build it
// with `gosd build --usb-gadget` (required so the board's USB port is in
// peripheral mode by the time this app runs).
package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jphastings/gosd/gadget"
)

// vendorID and productID are the Linux kernel's own g_serial gadget
// driver's placeholder USB IDs (NetChip Technology) — a widely recognized
// development pairing, not a USB-IF-assigned VID for a shipping product.
const (
	vendorID  = 0x0525
	productID = 0xa4a7

	ttyPath = "/dev/ttyGS0"

	// ttyOpenTimeout bounds how long this app waits for ttyPath to appear:
	// the device node exists only once the ACM driver has finished binding
	// to the UDC, which races this app's own startup.
	ttyOpenTimeout = 10 * time.Second
	ttyOpenRetry   = 200 * time.Millisecond

	// udcPollInterval is how often the USB controller's state is checked.
	// A host plugging in is a human-scale event, so a second is plenty.
	udcPollInterval = 1 * time.Second
)

func main() {
	g := gadget.Gadget{
		VendorID:     vendorID,
		ProductID:    productID,
		Manufacturer: "GoSD",
		Product:      "GoSD USB Serial",
		Serial:       "usbserial-example",
		Functions:    []gadget.Function{gadget.ACM{}},
	}
	if err := g.Apply(); err != nil {
		fmt.Fprintf(os.Stderr, "gosd usbserial: applying USB gadget failed: %v\n", err)
		os.Exit(1)
	}
	// This only runs on process exit, so there's nothing more the app can
	// do if the kernel refuses to unbind — but say so on the console:
	// gosd-init restarts /app, and a gadget whose configfs tree outlived
	// this process is exactly what makes the next Apply fail with EBUSY.
	defer func() {
		if err := g.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "gosd usbserial: tearing down the USB gadget failed, so its configfs state is still present: %v\n", err)
		}
	}()

	fmt.Println("gosd usbserial: gadget applied, waiting for", ttyPath)
	go reportUDCState(udcPollInterval)

	tty, err := openTTYWithRetry(ttyPath, ttyOpenTimeout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gosd usbserial: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = tty.Close() }()

	fmt.Println("gosd usbserial: echoing lines over", ttyPath)

	scanner := bufio.NewScanner(tty)
	for scanner.Scan() {
		if _, err := fmt.Fprintf(tty, "%s\n", scanner.Text()); err != nil {
			fmt.Fprintf(os.Stderr, "gosd usbserial: write failed: %v\n", err)
			os.Exit(1)
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "gosd usbserial: read failed: %v\n", err)
		os.Exit(1)
	}
}

// reportUDCState logs what the USB device controller believes is attached,
// once at startup and then on every change.
//
// Without this an unenumerated gadget is undiagnosable from the board. A
// device that has bound its ACM function looks identical whether a host is
// talking to it or the cable only carries power: /dev/ttyGS0 exists either
// way, and a GoSD device has no shell to go and look with. The controller's
// own state file is the one place that distinguishes them — "not attached"
// means no host or no data path, "configured" means a host has enumerated
// this gadget — which turns "it did not appear on my Mac" from a guess into
// a fact about which end is at fault.
func reportUDCState(every time.Duration) {
	var last string
	for {
		if state := udcState(); state != last {
			fmt.Println("gosd usbserial: USB controller state:", state)
			last = state
		}
		time.Sleep(every)
	}
}

// udcState reads every USB device controller's state, named, so a board with
// more than one says which is which.
func udcState() string {
	states, err := filepath.Glob("/sys/class/udc/*/state")
	if err != nil || len(states) == 0 {
		return "no USB device controller present"
	}
	var report []string
	for _, path := range states {
		name := filepath.Base(filepath.Dir(path))
		value, err := os.ReadFile(path)
		if err != nil {
			report = append(report, fmt.Sprintf("%s=unreadable (%v)", name, err))
			continue
		}
		report = append(report, fmt.Sprintf("%s=%s", name, strings.TrimSpace(string(value))))
	}
	return strings.Join(report, " ")
}

// openTTYWithRetry opens path, retrying on "not found" until timeout
// elapses — the ACM tty node appears asynchronously after Apply binds the
// UDC, not immediately.
func openTTYWithRetry(path string, timeout time.Duration) (*os.File, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		f, err := os.OpenFile(path, os.O_RDWR, 0)
		if err == nil {
			return f, nil
		}
		lastErr = err
		time.Sleep(ttyOpenRetry)
	}
	return nil, fmt.Errorf("opening %s timed out after %s: %w", path, timeout, lastErr)
}
