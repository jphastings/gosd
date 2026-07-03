package boot

import (
	"bytes"
	"testing"
)

func TestLoggerPrefixesAndFormats(t *testing.T) {
	var buf bytes.Buffer
	log := NewLogger(&buf)

	log.Printf("mounted %s at %s", "proc", "/proc")
	log.Printf("attempt %d of %d", 2, 5)

	want := "[gosd] mounted proc at /proc\n[gosd] attempt 2 of 5\n"
	if got := buf.String(); got != want {
		t.Fatalf("Printf output = %q, want %q", got, want)
	}
}
