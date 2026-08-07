package cloudflared

import (
	"strings"
	"testing"
)

func TestLineWriterLogsOneLinePerWrite(t *testing.T) {
	log := &testLog{}
	w := newLineWriter(log.Printf)

	if _, err := w.Write([]byte("tunnel connected\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got := log.snapshot()
	want := []string{"cloudflared: tunnel connected"}
	if len(got) != 1 || got[0] != want[0] {
		t.Fatalf("logged lines = %v, want %v", got, want)
	}
}

func TestLineWriterSplitsMultipleLinesInOneWrite(t *testing.T) {
	log := &testLog{}
	w := newLineWriter(log.Printf)

	if _, err := w.Write([]byte("line one\nline two\nline three\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got := log.snapshot()
	want := []string{"cloudflared: line one", "cloudflared: line two", "cloudflared: line three"}
	if len(got) != len(want) {
		t.Fatalf("logged lines = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestLineWriterHoldsPartialLineAcrossWrites(t *testing.T) {
	log := &testLog{}
	w := newLineWriter(log.Printf)

	if _, err := w.Write([]byte("connec")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if len(log.snapshot()) != 0 {
		t.Fatalf("logged a line before a newline arrived: %v", log.snapshot())
	}

	if _, err := w.Write([]byte("ted to edge\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got := log.snapshot()
	if len(got) != 1 || got[0] != "cloudflared: connected to edge" {
		t.Fatalf("logged lines = %v, want [\"cloudflared: connected to edge\"]", got)
	}
}

func TestLineWriterOverflowTruncatesWithNote(t *testing.T) {
	log := &testLog{}
	w := newLineWriter(log.Printf)

	overflowing := strings.Repeat("x", maxBufferedLine+100)
	if _, err := w.Write([]byte(overflowing)); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got := log.snapshot()
	if len(got) != 1 {
		t.Fatalf("logged lines = %v, want exactly 1", got)
	}
	if !strings.Contains(got[0], "[line truncated at 4096 bytes]") {
		t.Errorf("logged line %q does not carry a truncation note", got[0])
	}
	if !strings.HasPrefix(got[0], "cloudflared: "+strings.Repeat("x", maxBufferedLine)) {
		t.Errorf("truncated line does not start with the first %d bytes written", maxBufferedLine)
	}

	// The line eventually terminates normally; only the truncated prefix
	// should have been logged as its own line, and the remainder logs once
	// a newline finally arrives.
	if _, err := w.Write([]byte("tail\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got = log.snapshot()
	if len(got) != 2 || got[1] != "cloudflared: tail" {
		t.Fatalf("logged lines after the tail = %v, want a second line \"cloudflared: tail\"", got)
	}
}

func TestLineWriterInstancesAreIndependent(t *testing.T) {
	logA := &testLog{}
	logB := &testLog{}
	a := newLineWriter(logA.Printf)
	b := newLineWriter(logB.Printf)

	if _, err := a.Write([]byte("from a, no newline yet")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := b.Write([]byte("from b\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if got := logA.snapshot(); len(got) != 0 {
		t.Fatalf("writer a logged before its newline arrived: %v", got)
	}
	if got := logB.snapshot(); len(got) != 1 || got[0] != "cloudflared: from b" {
		t.Fatalf("writer b logged lines = %v, want [\"cloudflared: from b\"]", got)
	}

	if err := a.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := logA.snapshot(); len(got) != 1 || got[0] != "cloudflared: from a, no newline yet" {
		t.Fatalf("writer a after Close logged = %v, want its partial line flushed", got)
	}
	if got := logB.snapshot(); len(got) != 1 {
		t.Fatalf("writer b's log changed after writer a's Close: %v", got)
	}
}

func TestLineWriterCloseIsNoopWhenBufferEmpty(t *testing.T) {
	log := &testLog{}
	w := newLineWriter(log.Printf)

	if _, err := w.Write([]byte("already flushed\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if got := log.snapshot(); len(got) != 1 {
		t.Fatalf("Close logged an extra line: %v", got)
	}
}
