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
	// Best-effort: this only runs on process exit, and there's nothing
	// more the app can do if the kernel refuses to unbind.
	defer func() { _ = g.Close() }()

	fmt.Println("gosd usbserial: gadget applied, waiting for", ttyPath)

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
