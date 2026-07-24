// Command usbwebsite turns a GoSD board with onboard eMMC into a tiny
// self-contained website appliance that you edit by USB. On a standalone boot
// it serves the eMMC's contents as a static website over HTTP; plugged into a
// computer it presents that same eMMC as a removable USB drive, so you can drop
// or edit the site's files, then power it standalone again to serve them.
//
// It demonstrates gadget.MassStorage (sharing a block device over USB) on top
// of the emmc package (which reports the device backing its mount). The board
// must be built with `gosd build --usb-gadget` so its USB port is in peripheral
// mode, and must have onboard eMMC and a USB gadget controller — the Radxa Zero
// 3E is the board that has both today. Without an eMMC it logs that plainly and
// exits; without a USB controller it just serves.
//
// The USB-vs-website decision is made once per boot: presenting the drive and
// mounting it locally must never be live at the same time (the host writes raw
// blocks with no knowledge of our filesystem), so the app either hands the
// device to a connected computer or keeps it mounted to serve — never both.
//
// A board whose eMMC already holds other content (a vendor image, a prior
// project) needs explicit consent before this app claims it: set the
// WEBSITE_WIPE_EMMC gosd.toml [env] var (see docs/runtime.md's "App
// environment variables") to "yes" (or "1"/"true"/"on") to let it wipe and
// reformat that eMMC. Without consent it leaves the eMMC untouched, logs
// what to do about it, and idles rather than exiting — gosd-init restarts
// exited apps regardless of exit code, so exiting here would just crash-loop.
package main

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jphastings/gosd/emmc"
	"github.com/jphastings/gosd/gadget"
)

const (
	label      = "WEBSITE"
	mountpoint = "/storage"
	httpAddr   = ":80"

	// wipeConsentEnv is the gosd.toml [env] var (see docs/runtime.md's "App
	// environment variables") a user sets to let usbwebsite claim an eMMC
	// that already holds other content. Unset, the app only ever formats an
	// eMMC that's blank or already carries its own label.
	wipeConsentEnv = "WEBSITE_WIPE_EMMC"

	udcDir = "/sys/class/udc"

	// vendorID and productID are the Linux kernel's own g_mass_storage gadget
	// placeholder USB IDs (Linux Foundation) — a recognized development
	// pairing, not a USB-IF-assigned VID for a shipping product.
	vendorID  = 0x1d6b
	productID = 0x0104

	// hostSettle bounds how long we wait, after presenting the drive, for a
	// connected computer to enumerate and configure it. A computer does this in
	// well under a second; a power-only supply never will, so this also caps
	// how long a standalone boot spends probing before it falls back to
	// serving.
	hostSettle = 4 * time.Second
	hostPoll   = 200 * time.Millisecond

	readHeaderTimeout = 10 * time.Second
)

func main() {
	destructive := wipeConsented()
	res := <-emmc.FormatAndMount(label, mountpoint, destructive)
	if res.Err != nil {
		switch {
		case errors.Is(res.Err, emmc.ErrNoEMMC):
			fmt.Println("gosd usbwebsite: no onboard eMMC on this board; this example needs one (e.g. a Radxa Zero 3E)")
			return
		case !destructive && needsWipeConsent(res.Err):
			fmt.Printf("gosd usbwebsite: %v\n", res.Err)
			fmt.Printf("gosd usbwebsite: to let usbwebsite claim it, add %s = \"yes\" to the [env] table in gosd.toml on the GOSD-BOOT partition, then reboot\n", wipeConsentEnv)
			idleForever()
		default:
			fmt.Fprintf(os.Stderr, "gosd usbwebsite: %v\n", res.Err)
			os.Exit(1)
		}
	}
	fmt.Printf("gosd usbwebsite: %s ready at %s (device %s)\n", label, res.MountPoint, res.BlockDevice)

	if presentedAsDrive(res) {
		// A computer is editing the files; stay a drive until it is unplugged
		// and the board reboots. Serving now would fight the host for the
		// device.
		fmt.Println("gosd usbwebsite: computer attached — sharing the website storage as a USB drive")
		fmt.Println("gosd usbwebsite: edit the files, eject the drive, then power the board standalone to serve them")
		idleForever()
	}

	serveWebsite(res.MountPoint)
}

// wipeConsented reports whether the user has opted in, via wipeConsentEnv, to
// letting usbwebsite wipe and claim an eMMC that already holds other content.
func wipeConsented() bool {
	return isAffirmative(os.Getenv(wipeConsentEnv))
}

// isAffirmative recognizes the usual "yes" spellings for a boolean gosd.toml
// [env] value, case-insensitively; anything else, including unset or empty,
// means no — the safe default.
func isAffirmative(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// needsWipeConsent reports whether err is emmc.FormatAndMount's refusal to
// touch an eMMC that already holds other content — the case wipeConsentEnv
// unlocks. The emmc package exports no sentinel for this (only ErrNoEMMC), so
// this matches its message text instead; if that wording ever changes this
// simply falls through to the generic exit-1 handling below, which is still
// correct, just less specific.
func needsWipeConsent(err error) bool {
	return strings.Contains(err.Error(), "refusing to reformat")
}

// idleForever blocks forever without exiting, so gosd-init's automatic
// restart-on-exit (which applies regardless of exit code) doesn't
// crash-loop this app while it waits on outside action — a user setting an
// env var, or plugging in a computer. A bare `select {}` isn't safe for this:
// with no other goroutine able to wake it, the Go runtime treats that as a
// deadlock and panics instead of blocking.
func idleForever() {
	for {
		time.Sleep(time.Hour)
	}
}

// presentedAsDrive tries to hand the eMMC to a connected computer as a USB
// mass-storage drive. It returns true only if a computer actually enumerated
// and configured it. On every other outcome — no USB gadget controller, no
// cable, a power-only supply, or a setup error — it leaves (or restores) the
// eMMC mounted at res.MountPoint and returns false, so the caller serves the
// website instead.
func presentedAsDrive(res emmc.Result) bool {
	udc, err := firstUDC()
	if err != nil {
		fmt.Printf("gosd usbwebsite: not offering a USB drive (%v)\n", err)
		return false
	}
	if udcState(udc) == "not attached" {
		// No USB cable/VBUS at all: definitely standalone, so skip the probe
		// and serve straight away.
		return false
	}

	// Give up our mount of the device before exposing it: a mass-storage LUN
	// and a local mount of the same block device must never be live at once.
	if err := emmc.Unmount(res.MountPoint); err != nil {
		fmt.Printf("gosd usbwebsite: could not release %s to share it (%v); serving instead\n", res.MountPoint, err)
		return false
	}

	g := &gadget.Gadget{
		VendorID:     vendorID,
		ProductID:    productID,
		Manufacturer: "GoSD",
		Product:      "GoSD Website Storage",
		Serial:       "usbwebsite-example",
		Functions: []gadget.Function{
			gadget.MassStorage{Path: res.BlockDevice, Removable: true},
		},
	}
	if err := g.Apply(); err != nil {
		fmt.Printf("gosd usbwebsite: presenting the USB drive failed (%v); serving instead\n", err)
		remount()
		return false
	}

	if awaitConfigured(udc, hostSettle) {
		return true
	}

	// VBUS present but nothing enumerated us (e.g. a phone charger): tear the
	// drive back down, remount, and serve.
	fmt.Println("gosd usbwebsite: no computer enumerated the drive; serving the website instead")
	_ = g.Close()
	remount()
	return false
}

// serveWebsite serves dir as a static site forever. A freshly formatted eMMC
// has no index.html, so it drops in a starter page first.
func serveWebsite(dir string) {
	ensureStarterPage(dir)
	fmt.Printf("gosd usbwebsite: serving %s on %s\n", dir, httpAddr)

	srv := &http.Server{
		Addr:              httpAddr,
		Handler:           http.FileServer(http.Dir(dir)),
		ReadHeaderTimeout: readHeaderTimeout,
	}
	if err := srv.ListenAndServe(); err != nil {
		fmt.Fprintf(os.Stderr, "gosd usbwebsite: %v\n", err)
		os.Exit(1)
	}
}

// ensureStarterPage writes a placeholder index.html when the site has none, so
// a brand-new board serves something that explains how to add real content.
func ensureStarterPage(dir string) {
	index := filepath.Join(dir, "index.html")
	if _, err := os.Stat(index); err == nil {
		return // the user's own content is already here
	}
	const starter = `<!doctype html>
<title>GoSD usbwebsite</title>
<h1>It works!</h1>
<p>This page is served by a GoSD board from its onboard eMMC.</p>
<p>Plug the board into a computer over USB and it appears as a removable drive
labelled WEBSITE. Replace this index.html (and add whatever else you like),
eject the drive, then power the board on its own again to serve your site.</p>
`
	if err := os.WriteFile(index, []byte(starter), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "gosd usbwebsite: could not write the starter page: %v\n", err)
	}
}

// firstUDC returns the board's first USB peripheral controller under
// /sys/class/udc, or an error naming why gadget mode is unavailable.
func firstUDC() (string, error) {
	entries, err := os.ReadDir(udcDir)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", udcDir, err)
	}
	if len(entries) == 0 {
		return "", fmt.Errorf("no USB peripheral controller under %s; build with `gosd build --usb-gadget`", udcDir)
	}
	return entries[0].Name(), nil
}

// udcState reads the controller's USB device state ("not attached", "powered",
// "configured", …), or "" if it can't be read.
func udcState(udc string) string {
	raw, err := os.ReadFile(udcDir + "/" + udc + "/state")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

// awaitConfigured polls the controller until a host has configured the gadget
// (the USB "configured" state) or timeout elapses.
func awaitConfigured(udc string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if udcState(udc) == "configured" {
			return true
		}
		time.Sleep(hostPoll)
	}
	return false
}

// remount restores the eMMC mount after the drive is torn down. It is
// idempotent: FormatAndMount only remounts, never reformats, an eMMC that
// already carries this app's label.
func remount() {
	if res := <-emmc.FormatAndMount(label, mountpoint, false); res.Err != nil {
		fmt.Fprintf(os.Stderr, "gosd usbwebsite: remounting %s failed: %v\n", mountpoint, res.Err)
	}
}
