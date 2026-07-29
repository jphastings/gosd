package main

import (
	"encoding/binary"
	"math"
	"testing"
)

var testFormat = format{rate: 48000, channels: 2}

// samples decodes channel 0 out of interleaved S16_LE frames.
func samples(t *testing.T, pcm []byte, f format) []int16 {
	t.Helper()
	if len(pcm)%f.frameBytes() != 0 {
		t.Fatalf("%d bytes is not a whole number of %d-byte frames", len(pcm), f.frameBytes())
	}
	out := make([]int16, 0, len(pcm)/f.frameBytes())
	for i := 0; i < len(pcm); i += f.frameBytes() {
		out = append(out, int16(binary.LittleEndian.Uint16(pcm[i:])))
	}
	return out
}

func peak(s []int16) int16 {
	var max int16
	for _, v := range s {
		if v < 0 {
			v = -v
		}
		if v > max {
			max = v
		}
	}
	return max
}

func zeroCrossings(s []int16) int {
	n := 0
	for i := 1; i < len(s); i++ {
		if (s[i-1] < 0) != (s[i] < 0) {
			n++
		}
	}
	return n
}

func TestChimeIsTwoNotesThatDecay(t *testing.T) {
	pcm := chime(testFormat)
	notePerNote := int(chimeNote.Seconds() * float64(testFormat.rate))
	if want := 2 * notePerNote * testFormat.frameBytes(); len(pcm) != want {
		t.Fatalf("chime is %d bytes, want %d (two %s notes)", len(pcm), want, chimeNote)
	}

	all := samples(t, pcm, testFormat)
	window := testFormat.rate / 50 // 20ms
	for note := 0; note < 2; note++ {
		start := note * notePerNote
		head := peak(all[start : start+window])
		tail := peak(all[start+notePerNote-window : start+notePerNote])
		if head <= tail {
			t.Errorf("note %d peaks at %d at its start and %d at its end; it should decay", note, head, tail)
		}
		if tail == 0 {
			t.Errorf("note %d has decayed to silence before it ends", note)
		}
	}
}

func TestChimeNotesRiseInPitch(t *testing.T) {
	all := samples(t, chime(testFormat), testFormat)
	notePerNote := len(all) / 2
	// Count over the loud first half of each note, where the crossings are
	// unambiguous.
	first := zeroCrossings(all[:notePerNote/2])
	second := zeroCrossings(all[notePerNote : notePerNote+notePerNote/2])
	if second <= first {
		t.Errorf("second note has %d zero crossings, first has %d; the second should be higher", second, first)
	}
}

func TestSweepRisesInPitchAndStartsAndEndsQuietly(t *testing.T) {
	all := samples(t, sweep(testFormat), testFormat)
	if want := int(sweepTone.Seconds() * float64(testFormat.rate)); len(all) != want {
		t.Fatalf("sweep is %d frames, want %d", len(all), want)
	}

	window := testFormat.rate / 10 // 100ms
	if head, tail := zeroCrossings(all[:window]), zeroCrossings(all[len(all)-window:]); tail <= head*2 {
		t.Errorf("sweep has %d zero crossings in its first 100ms and %d in its last; it should climb several octaves", head, tail)
	}
	// The raised-cosine ramps exist so the tone doesn't click: the very first
	// and last frames should be near silence.
	quiet := int16(fullScale / 100)
	if got := peak(all[:testFormat.rate/1000]); got > quiet {
		t.Errorf("sweep opens at amplitude %d, want under %d (a click)", got, quiet)
	}
	if got := peak(all[len(all)-testFormat.rate/1000:]); got > quiet {
		t.Errorf("sweep closes at amplitude %d, want under %d (a click)", got, quiet)
	}
}

func TestNothingClips(t *testing.T) {
	limit := int16(math.Round(headroom*fullScale)) + 1
	for name, pcm := range map[string][]byte{"chime": chime(testFormat), "sweep": sweep(testFormat)} {
		if got := peak(samples(t, pcm, testFormat)); got > limit {
			t.Errorf("%s peaks at %d, want no more than %d", name, got, limit)
		}
	}
}

func TestChannelsCarryTheSameSignal(t *testing.T) {
	pcm := chime(testFormat)
	for i := 0; i < len(pcm); i += testFormat.frameBytes() {
		left := binary.LittleEndian.Uint16(pcm[i:])
		right := binary.LittleEndian.Uint16(pcm[i+2:])
		if left != right {
			t.Fatalf("frame %d has %d left and %d right; the chime is mono in both channels", i/testFormat.frameBytes(), left, right)
		}
	}
}

// A different rate must change the frame count but not the pitch, which is
// the one thing a phase-accumulating oscillator can get wrong.
func TestRateChangesLengthNotPitch(t *testing.T) {
	slow := format{rate: 24000, channels: 1}
	fast := format{rate: 48000, channels: 1}
	slowTone := samples(t, sweep(slow), slow)
	fastTone := samples(t, sweep(fast), fast)

	if len(fastTone) != 2*len(slowTone) {
		t.Errorf("48 kHz sweep is %d frames and 24 kHz is %d; want double", len(fastTone), len(slowTone))
	}
	slowCross := zeroCrossings(slowTone)
	fastCross := zeroCrossings(fastTone)
	if ratio := float64(fastCross) / float64(slowCross); ratio < 0.9 || ratio > 1.1 {
		t.Errorf("zero crossings differ by %.2fx between rates (%d vs %d); the pitch should not depend on the sample rate", ratio, fastCross, slowCross)
	}
}
