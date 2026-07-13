// Command sattrack is a GoSD example that turns a board with an HDMI
// display into a live satellite tracker: a fullscreen NASA Blue Marble
// world map with the chosen satellite's current position, its ground track
// over the past 30 minutes (solid, fading out at the oldest tip) and the
// coming 30 minutes (dashed), and its name, updated once per second with
// partial redraws over DRM/KMS dumb buffers.
//
// GoSD's stock kernels cut DRM entirely, so this example ships a
// custom-kernel recipe under kernel/ for `gosd build-kernel` - see
// README.md for the full walkthrough (pi-zero-w over real HDMI, qemu-virt
// in a host window via `gosd run --display`) and docs/custom-kernels.md
// for the mechanism.
package main

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"image"
	"image/jpeg"
	"io/fs"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	// GoSD images ship no /etc/ssl CA bundle, so crypto/x509 has no system
	// roots to verify the TLE API's certificate against; this registers
	// the Mozilla root store as the fallback, the standard pure-Go answer
	// for HTTPS from minimal images.
	_ "golang.org/x/crypto/x509roots/fallback"
)

// bluemarble.jpg is NASA's Blue Marble Next Generation (December 2004,
// world.topo.bathy, https://visibleearth.nasa.gov/images/73909), a
// public-domain NASA image, downscaled to 2048x1024 - attribution and
// source details in README.md.
//
//go:embed bluemarble.jpg
var blueMarbleJPEG []byte

// blackmarble.jpg is NASA's Black Marble (Earth at Night 2016 composite,
// https://visibleearth.nasa.gov/images/144898), a public-domain NASA
// image, downscaled to 2048x1024 - attribution and source details in
// README.md. It is the night-side base texture; the Blue Marble shows
// through where the sun is up.
//
//go:embed blackmarble.jpg
var blackMarbleJPEG []byte

// defaultNoradID is the satellite tracked when SATTRACK_NORAD_ID isn't
// set (via the environment or gosd.toml's [env] table).
const defaultNoradID = "68498"

// clockPlausibleAfter guards SGP4 propagation against the epoch clock a
// board without an RTC boots with: gosd-init's SNTP sync can land after
// the app starts, and propagating a TLE from 1970 is nonsense.
var clockPlausibleAfter = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

// displayRetryEvery paces retries when no DRM display exists (stock
// kernel, cable unplugged): a slow steady retry instead of exiting keeps
// gosd-init's supervisor from restart-churning the whole app.
const displayRetryEvery = 60 * time.Second

func main() {
	// No prefix: gosd-init already tags this app's console output by name.
	log.SetFlags(0)

	noradID := os.Getenv("SATTRACK_NORAD_ID")
	if noradID == "" {
		noradID = defaultNoradID
	}
	log.Printf("tracking NORAD id %s (override with SATTRACK_NORAD_ID)", noradID)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dayImg, err := jpeg.Decode(bytes.NewReader(blueMarbleJPEG))
	if err != nil {
		log.Fatalf("decoding the embedded Blue Marble map failed: %v", err)
	}
	nightImg, err := jpeg.Decode(bytes.NewReader(blackMarbleJPEG))
	if err != nil {
		log.Fatalf("decoding the embedded Black Marble map failed: %v", err)
	}

	src := newTLESource(noradID)
	go src.run(ctx)

	waitForPlausibleClock(ctx)

	log.Print("waiting for the first TLE fetch...")
	var cur sat
	select {
	case cur = <-src.updates:
	case <-ctx.Done():
		return
	}

	for ctx.Err() == nil {
		d, err := openDisplay()
		if err != nil {
			logDisplayError(err)
			select {
			case <-time.After(displayRetryEvery):
			case <-ctx.Done():
			}
			continue
		}
		log.Printf("display: %dx%d", d.Size().X, d.Size().Y)

		err = drive(ctx, d, dayImg, nightImg, src, &cur)
		_ = d.Close()
		if err != nil {
			log.Printf("display output failed (%v); reinitializing", err)
		}
	}
}

// logDisplayError explains the two very different reasons openDisplay
// fails: no KMS device at all (stock GoSD kernel - the fix is this
// example's custom kernel) versus a device the process may not touch.
func logDisplayError(err error) {
	if errors.Is(err, fs.ErrPermission) {
		log.Printf("no usable display: %v - the app needs read/write access to /dev/dri/card* (run as root or in the video group); retrying every %s", err, displayRetryEvery)
		return
	}
	log.Printf("no usable display (%v) - GoSD's stock kernels have no DRM support; build this example's custom kernel (see examples/sattrack/README.md and docs/custom-kernels.md) and check the HDMI cable; retrying every %s", err, displayRetryEvery)
}

// drive owns the display until ctx ends or a flush fails: a full paint,
// then 1s ticks that repaint and flush only what changed. A TLE refresh
// swaps the element set and forces one full repaint (both track windows
// move); a propagation failure asks the TLE source for a fresh fetch and
// keeps the screen as-is rather than tearing it down.
func drive(ctx context.Context, d *display, dayImg, nightImg image.Image, src *tleSource, cur *sat) error {
	r, err := newRenderer(d.Size(), dayImg, nightImg)
	if err != nil {
		return err
	}
	r.setName(cur.name)

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		f, err := computeFrame(cur.sgp4, time.Now())
		if err != nil {
			log.Printf("%v; requesting a fresh TLE", err)
			src.kick()
		} else {
			if err := d.Flush(r.back, r.render(f)); err != nil {
				return err
			}
		}

		select {
		case <-ctx.Done():
			return nil
		case s := <-src.updates:
			*cur = s
			r.setName(s.name)
			r.invalidate()
		case <-ticker.C:
		}
	}
}

// waitForPlausibleClock blocks until the wall clock has clearly been set
// (SNTP may land after the app starts; GoSD boards have no RTC).
func waitForPlausibleClock(ctx context.Context) {
	logged := false
	for time.Now().Before(clockPlausibleAfter) {
		if !logged {
			log.Print("waiting for the clock to be set (SNTP) before propagating orbits...")
			logged = true
		}
		select {
		case <-time.After(time.Second):
		case <-ctx.Done():
			return
		}
	}
}
