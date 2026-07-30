// Command chime plays a boot chime and then a periodic test tone out of a
// GoSD board's audio output — HDMI where the board has it, the analog jack
// where it has one — using the gosd sound package, which talks the kernel's
// ALSA PCM interface directly. There is no alsa-lib in a GoSD image.
//
// GoSD's stock kernels have no sound at all, so this example ships the
// `gosd build-kernel` recipes that compile it back in: see README.md and
// docs/sound.md.
package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jphastings/gosd/sound"
)

const (
	// retryEvery is how often to look for an audio device again when there
	// isn't one. Deliberately unhurried: the usual cause is a kernel without
	// sound compiled in, which no amount of retrying will fix.
	retryEvery = 60 * time.Second
	// defaultEvery is how often the test tone plays.
	defaultEvery = 30 * time.Second
)

func main() {
	log.SetFlags(0)

	every := defaultEvery
	if raw := os.Getenv("CHIME_EVERY"); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil || d <= 0 {
			log.Fatalf("CHIME_EVERY=%q is not a positive Go duration (try 45s or 5m)", raw)
		}
		every = d
	}
	prefer, err := preferredOutput(os.Getenv("CHIME_OUTPUT"))
	if err != nil {
		log.Fatal(err)
	}
	opts := sound.Options{Path: os.Getenv("CHIME_DEVICE"), Prefer: prefer}

	log.Printf("chime: boot chime, then a test tone every %s (override with CHIME_EVERY)", every)

	for {
		dev, err := sound.OpenWith(opts)
		if err != nil {
			logNoAudio(err)
			time.Sleep(retryEvery)
			continue
		}
		log.Printf("playing to %s at %s", dev.Name(), dev.Format())
		if err := play(dev, every); err != nil {
			log.Printf("playback failed (%v); reopening the audio device", err)
		}
		if err := dev.Close(); err != nil {
			log.Printf("closing the audio device: %v", err)
		}
	}
}

// preferredOutput reads CHIME_OUTPUT. The default prefers HDMI, which is the
// output every GoSD board with any audio hardware at all can reach; a ROCK 4SE
// built with the analog-only kernel recipe has no HDMI PCM to find, so it lands
// on its jack without being told to.
func preferredOutput(raw string) (sound.Output, error) {
	switch raw {
	case "", "hdmi":
		return sound.HDMI, nil
	case "analog":
		return sound.Analog, nil
	case "any":
		return sound.Any, nil
	default:
		return sound.Any, fmt.Errorf("CHIME_OUTPUT=%q is not one of hdmi, analog or any", raw)
	}
}

// play sounds the chime, then the test tone on a ticker, until something goes
// wrong. It returns rather than exiting so main can reopen the device: this
// app deliberately never exits, so gosd-init's supervisor has nothing to
// restart-churn.
func play(dev sound.Device, every time.Duration) error {
	f := dev.Format()
	if err := dev.Play(chime(f)); err != nil {
		return err
	}
	tick := time.NewTicker(every)
	defer tick.Stop()
	tone := sweep(f)
	for range tick.C {
		if err := dev.Play(tone); err != nil {
			return err
		}
	}
	return nil
}

// logNoAudio explains the two very different reasons there might be no audio
// device, because the fix for each is completely different. sound.Open's own
// error already covers "this kernel has no sound"; permissions it cannot know
// about.
func logNoAudio(err error) {
	if errors.Is(err, os.ErrPermission) {
		log.Printf("no usable audio device: %v - the app needs read/write access to /dev/snd/pcm*; retrying every %s", err, retryEvery)
		return
	}
	log.Printf("no usable audio device: %v - see examples/chime/README.md; retrying every %s", err, retryEvery)
}
