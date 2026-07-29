package main

import (
	"errors"
	"testing"
	"time"
)

// fakeSink records what would have been played, and can be told to fail after
// a given number of writes.
type fakeSink struct {
	played   [][]byte
	failAt   int
	failWith error
}

func (f *fakeSink) Play(pcm []byte) error {
	f.played = append(f.played, pcm)
	if f.failAt > 0 && len(f.played) >= f.failAt {
		return f.failWith
	}
	return nil
}

func (f *fakeSink) Format() format { return testFormat }
func (f *fakeSink) Name() string   { return "fake" }
func (f *fakeSink) Close() error   { return nil }

func TestPlaySoundsTheChimeFirstThenTones(t *testing.T) {
	boom := errors.New("device went away")
	s := &fakeSink{failAt: 3, failWith: boom}

	if err := play(s, time.Millisecond); !errors.Is(err, boom) {
		t.Fatalf("play returned %v, want the sink's error", err)
	}
	if len(s.played) != 3 {
		t.Fatalf("played %d sounds, want 3 (a chime and two tones)", len(s.played))
	}
	if want := len(chime(testFormat)); len(s.played[0]) != want {
		t.Errorf("first sound is %d bytes, want the %d-byte chime", len(s.played[0]), want)
	}
	if want := len(sweep(testFormat)); len(s.played[1]) != want || len(s.played[2]) != want {
		t.Errorf("later sounds are %d and %d bytes, want the %d-byte test tone", len(s.played[1]), len(s.played[2]), want)
	}
}

// A device that fails on the very first write must not swallow the error: main
// relies on play returning so it can reopen the device.
func TestPlayReturnsIfTheChimeFails(t *testing.T) {
	boom := errors.New("EPIPE forever")
	s := &fakeSink{failAt: 1, failWith: boom}

	if err := play(s, time.Hour); !errors.Is(err, boom) {
		t.Fatalf("play returned %v, want the sink's error", err)
	}
	if len(s.played) != 1 {
		t.Errorf("played %d sounds, want just the failed chime", len(s.played))
	}
}
