package main

import (
	"math"
	"time"
)

// Everything this example plays is synthesised here: a chime is a couple of
// decaying sine notes and a test tone is a swept sine, which is a few lines of
// math and no bundled asset, no licence question, and no image-size cost.

// headroom keeps the loudest sample well inside full scale, so nothing clips
// through whatever gain stage is on the other end of an HDMI cable.
const headroom = 0.5

// fullScale is the largest magnitude a signed 16-bit sample can carry.
const fullScale = 32767

// envelope shapes amplitude over the life of a sound; freq gives the
// instantaneous frequency in Hz. Both take the time since the sound started
// and its total duration, in seconds.
type envelope func(t, total float64) float64
type frequency func(t, total float64) float64

// render synthesises one sound as interleaved S16_LE frames, accumulating
// phase so a changing frequency stays continuous (a naive sin(2*pi*f(t)*t)
// audibly clicks and sweeps at twice the intended rate).
func render(f format, dur time.Duration, freq frequency, env envelope) []byte {
	total := dur.Seconds()
	frames := int(float64(f.rate) * total)
	out := make([]byte, 0, frames*f.frameBytes())
	phase := 0.0
	for i := 0; i < frames; i++ {
		t := float64(i) / float64(f.rate)
		phase += 2 * math.Pi * freq(t, total) / float64(f.rate)
		sample := int16(math.Round(env(t, total) * math.Sin(phase) * fullScale))
		for c := 0; c < f.channels; c++ {
			out = append(out, byte(uint16(sample)), byte(uint16(sample)>>8))
		}
	}
	return out
}

// fixed is a constant frequency.
func fixed(hz float64) frequency {
	return func(float64, float64) float64 { return hz }
}

// glide sweeps exponentially from one frequency to another, which is how ears
// hear "evenly rising" — a linear sweep spends most of its time sounding high.
func glide(from, to float64) frequency {
	return func(t, total float64) float64 {
		return from * math.Pow(to/from, t/total)
	}
}

// pluck decays exponentially from full amplitude, like something struck.
func pluck(halfLife float64) envelope {
	return func(t, _ float64) float64 {
		return headroom * math.Exp2(-t/halfLife)
	}
}

// fade holds a steady level with short raised-cosine ramps at each end, so the
// tone starts and stops without a click.
func fade(ramp float64) envelope {
	return func(t, total float64) float64 {
		gain := headroom
		if t < ramp {
			gain *= 0.5 - 0.5*math.Cos(math.Pi*t/ramp)
		}
		if remaining := total - t; remaining < ramp {
			gain *= 0.5 - 0.5*math.Cos(math.Pi*remaining/ramp)
		}
		return gain
	}
}

// chimeNotes is a rising two-note figure (A5 then D6) — short, unmistakably
// deliberate, and high enough to survive a small monitor speaker.
var chimeNotes = []float64{880.00, 1174.66}

const (
	chimeNote = 350 * time.Millisecond
	sweepTone = 1200 * time.Millisecond
)

// chime renders the boot chime.
func chime(f format) []byte {
	var out []byte
	for _, hz := range chimeNotes {
		out = append(out, render(f, chimeNote, fixed(hz), pluck(0.12))...)
	}
	return out
}

// sweep renders the periodic test tone: four octaves of rising sine, which
// makes a dead tweeter or a resampling problem obvious by ear.
func sweep(f format) []byte {
	return render(f, sweepTone, glide(220, 3520), fade(0.03))
}
