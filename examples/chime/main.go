// Command chime plays a boot chime and then a periodic test tone out of a
// GoSD board's audio output — HDMI where the board has it, the analog jack
// where it has one — by talking the kernel's ALSA PCM ioctl interface
// directly. There is no alsa-lib in a GoSD image to talk to.
//
// GoSD's stock kernels have no sound at all, so this example ships the
// `gosd build-kernel` recipe that compiles it back in: see README.md and
// docs/custom-kernels.md.
package main

import (
	"errors"
	"log"
	"os"
	"time"
)

// sink is one configured audio output. The seam exists so the timing and
// degradation logic below is testable on a machine with no /dev/snd (i.e. the
// one this was written on).
type sink interface {
	// Play writes interleaved S16_LE frames and returns once they have
	// finished playing.
	Play(pcm []byte) error
	Format() format
	Name() string
	Close() error
}

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
	want := os.Getenv("CHIME_DEVICE")

	log.Printf("chime: boot chime, then a test tone every %s (override with CHIME_EVERY)", every)

	for {
		s, err := openSink(want)
		if err != nil {
			logNoAudio(err)
			time.Sleep(retryEvery)
			continue
		}
		f := s.Format()
		log.Printf("playing to %s at %d Hz, %d channels", s.Name(), f.rate, f.channels)
		if err := play(s, every); err != nil {
			log.Printf("playback failed (%v); reopening the audio device", err)
		}
		if err := s.Close(); err != nil {
			log.Printf("closing the audio device: %v", err)
		}
	}
}

// play sounds the chime, then the test tone on a ticker, until something goes
// wrong. It returns rather than exiting so main can reopen the device: this
// app deliberately never exits, so gosd-init's supervisor has nothing to
// restart-churn.
func play(s sink, every time.Duration) error {
	f := s.Format()
	if err := s.Play(chime(f)); err != nil {
		return err
	}
	tick := time.NewTicker(every)
	defer tick.Stop()
	tone := sweep(f)
	for range tick.C {
		if err := s.Play(tone); err != nil {
			return err
		}
	}
	return nil
}

// logNoAudio explains the two very different reasons there might be no audio
// device, because the fix for each is completely different.
func logNoAudio(err error) {
	if errors.Is(err, os.ErrPermission) {
		log.Printf("no usable audio device: %v - the app needs read/write access to /dev/snd/pcm*; retrying every %s", err, retryEvery)
		return
	}
	log.Printf("no usable audio device (%v) - GoSD's stock kernels have no sound support; build this example's custom kernel (see examples/chime/README.md and docs/custom-kernels.md), and remember HDMI audio only appears when a display is connected at boot; retrying every %s", err, retryEvery)
}
