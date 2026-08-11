package consoletail

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"testing"
)

func TestUnderCapacityReturnsWholeUnmarked(t *testing.T) {
	b := NewSize(64)

	if _, err := b.Write([]byte("hello\nworld\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if got := b.String(); got != "hello\nworld\n" {
		t.Fatalf("String() = %q, want %q", got, "hello\nworld\n")
	}
}

func TestWriteWithNoNewlinesAtAll(t *testing.T) {
	b := NewSize(64)
	in := "no newlines here at all, just a partial log line"

	if _, err := b.Write([]byte(in)); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if got := b.String(); got != in {
		t.Fatalf("String() = %q, want %q (unmarked, verbatim: nothing was ever evicted)", got, in)
	}
}

// TestOverCapacityDropsPartialLineAndMarks pins the exact byte-for-byte
// contract, worked out by hand: 20 lines of "lineNN\n" (7 bytes each, 140
// bytes total) into a 32-byte buffer keeps only the last 32 bytes, which
// lands mid-word inside "line15" — so that fragment must be dropped
// entirely rather than surviving as "e15\n...", and the result starts
// cleanly at the next full line, "line16\n".
func TestOverCapacityDropsPartialLineAndMarks(t *testing.T) {
	b := NewSize(32)

	var all strings.Builder
	for i := range 20 {
		fmt.Fprintf(&all, "line%02d\n", i)
	}
	if _, err := b.Write([]byte(all.String())); err != nil {
		t.Fatalf("Write: %v", err)
	}

	want := DroppedMarker + "line16\nline17\nline18\nline19\n"
	if got := b.String(); got != want {
		t.Fatalf("String() =\n%q\nwant\n%q", got, want)
	}
	if strings.Contains(b.String(), "line15") {
		t.Errorf("result contains a fragment of the dropped line: %q", b.String())
	}
}

// wantSingleWriteTail is an independent reference computation of what a
// single Write call larger than capacity should retain: the last capacity
// bytes, with any leading partial line dropped. Used as an oracle rather
// than a mirror of the implementation.
func wantSingleWriteTail(all []byte, capacity int) []byte {
	tail := all
	if len(tail) > capacity {
		tail = tail[len(tail)-capacity:]
	}
	if len(tail) == len(all) {
		return tail
	}
	idx := bytes.IndexByte(tail, '\n')
	if idx < 0 {
		return nil
	}
	return tail[idx+1:]
}

func TestSingleWriteFarLargerThanCapacity(t *testing.T) {
	const capacity = 100

	var all strings.Builder
	for i := range 2000 {
		fmt.Fprintf(&all, "line%04d\n", i)
	}
	input := []byte(all.String())

	b := NewSize(capacity)
	n, err := b.Write(input)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != len(input) {
		t.Fatalf("Write reported n=%d, want %d (the whole input, even though most is discarded)", n, len(input))
	}

	want := string(DroppedMarker) + string(wantSingleWriteTail(input, capacity))
	if got := b.String(); got != want {
		t.Fatalf("String() =\n%q\nwant\n%q", got, want)
	}
	if got := len(b.Bytes()); got > capacity+len(DroppedMarker) {
		t.Fatalf("retained %d bytes, want at most capacity+marker (%d)", got, capacity+len(DroppedMarker))
	}
}

func TestManySmallWrites(t *testing.T) {
	const capacity = 100
	b := NewSize(capacity)

	var last string
	for i := range 500 {
		last = fmt.Sprintf("item%03d\n", i)
		if _, err := b.Write([]byte(last)); err != nil {
			t.Fatalf("Write %d: %v", i, err)
		}
		if got := len(b.Bytes()); got > capacity+len(DroppedMarker) {
			t.Fatalf("after write %d, retained %d bytes, want at most capacity+marker (%d)", i, got, capacity+len(DroppedMarker))
		}
	}

	got := b.String()
	if !strings.HasPrefix(got, DroppedMarker) {
		t.Fatalf("String() = %q, want it marked as truncated", got)
	}
	if !strings.HasSuffix(got, last) {
		t.Fatalf("String() = %q, want it to end with the most recent write %q", got, last)
	}
}

func TestConcurrentWrites(t *testing.T) {
	const capacity = 256
	b := NewSize(capacity)

	var wg sync.WaitGroup
	writer := func(prefix string) {
		defer wg.Done()
		for i := range 200 {
			if _, err := fmt.Fprintf(b, "%s-%03d\n", prefix, i); err != nil {
				t.Errorf("Write: %v", err)
			}
		}
	}

	wg.Add(2)
	go writer("stdout")
	go writer("stderr")
	wg.Wait()

	if got := len(b.Bytes()); got > capacity+len(DroppedMarker) {
		t.Fatalf("retained %d bytes after concurrent writes, want at most capacity+marker (%d)", got, capacity+len(DroppedMarker))
	}
}
