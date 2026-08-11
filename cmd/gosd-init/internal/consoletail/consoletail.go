// Package consoletail implements a bounded byte buffer that retains only
// the last N bytes written to it, for gosd-init to keep a copy of a
// supervised app's console output so a crash report can carry it (bean
// gosd-s9uq, epic gosd-47z3).
//
// It is a close relative of cmd/gosd-init/internal/logwriter, which already
// solves the same "don't grow PID 1's memory on pathological input"
// problem for a different job: prefixed, line-split live logging of
// supervised children like cloudflared and tsfunnel. Buffer has no prefix
// and does no per-line logging — it just retains raw bytes for later
// rendering into a crash report.
package consoletail

import (
	"bytes"
	"io"
	"sync"
)

var _ io.Writer = (*Buffer)(nil)

// DefaultCapacity is how many bytes of console output a Buffer retains
// unless NewSize specifies otherwise. A Go panic prints every goroutine's
// stack, so this needs to comfortably hold a real one; it is still
// negligible next to the smallest board in the fleet (512MB).
const DefaultCapacity = 64 * 1024

// DroppedMarker prefixes a Buffer's content whenever earlier output has
// been discarded, so a reader of the rendered report knows they're looking
// at a tail rather than the whole run.
const DroppedMarker = "(earlier output dropped)\n"

// Buffer is an io.Writer that retains only the last N bytes written to it
// (DefaultCapacity unless constructed with NewSize). It never grows past
// its capacity regardless of input: an app that writes megabytes with no
// newline at all must not grow PID 1's memory on a board with as little as
// 512MB — the same concern logwriter.MaxBufferedLine already exists for.
// Excess is discarded from the FRONT, because a crash report needs the
// tail: whatever ran right up to the crash, not whatever happened first.
//
// One Buffer is written by both a supervised app's stdout and stderr
// concurrently in production (both tee into the same tail alongside the
// console), so Write is mutex-guarded.
type Buffer struct {
	capacity int

	mu        sync.Mutex
	buf       []byte
	truncated bool
}

// New creates a Buffer retaining the last DefaultCapacity bytes written to
// it.
func New() *Buffer {
	return NewSize(DefaultCapacity)
}

// NewSize creates a Buffer retaining the last capacity bytes written to it.
func NewSize(capacity int) *Buffer {
	return &Buffer{capacity: capacity}
}

// Write implements io.Writer. It always reports success and never returns
// an error: retention here is best-effort diagnostic capture and must never
// itself fail (or block) whatever else the app's stdout/stderr is teed to.
//
// Once a write forces bytes out of the front of the buffer, Write also
// drops the buffer's new leading partial line (see dropLeadingPartialLine)
// and marks the content as truncated so a later read carries DroppedMarker.
func (b *Buffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	n := len(p)
	truncatedThisCall := false

	// A write bigger than the whole buffer determines the entire retained
	// tail by itself; slice it down before touching buf at all, so a
	// single megabytes-sized write is never copied into buf in full only
	// to be discarded a moment later.
	if len(p) > b.capacity {
		p = p[len(p)-b.capacity:]
		truncatedThisCall = true
	}

	if room := b.capacity - len(b.buf); len(p) > room {
		skip := len(p) - room
		b.buf = append(b.buf[:0], b.buf[skip:]...)
		truncatedThisCall = true
	}
	b.buf = append(b.buf, p...)

	if truncatedThisCall {
		b.truncated = true
		b.dropLeadingPartialLine()
	}

	return n, nil
}

// dropLeadingPartialLine removes the buffer's leading partial line
// immediately after an eviction, rather than leaving a fragment at the
// front. This is not just for readability: the epic's redaction pass
// (gosd-m6py) matches whole secret strings against the tail, so a secret
// whose bytes straddle the truncation boundary would otherwise survive as
// an unredactable fragment. If the remaining buffer has no newline at all,
// every byte in it belongs to that same unterminated leading line, so it is
// dropped in full rather than kept as a guess. Callers must hold b.mu.
func (b *Buffer) dropLeadingPartialLine() {
	idx := bytes.IndexByte(b.buf, '\n')
	if idx < 0 {
		b.buf = b.buf[:0]
		return
	}
	b.buf = b.buf[:copy(b.buf, b.buf[idx+1:])]
}

// Bytes returns the retained content for rendering into a crash report,
// prefixed with DroppedMarker when earlier output was discarded. The
// returned slice is a copy: callers may retain or mutate it freely.
func (b *Buffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.truncated {
		out := make([]byte, len(b.buf))
		copy(out, b.buf)
		return out
	}

	out := make([]byte, 0, len(DroppedMarker)+len(b.buf))
	out = append(out, DroppedMarker...)
	out = append(out, b.buf...)
	return out
}

// String returns the same content as Bytes, as a string.
func (b *Buffer) String() string {
	return string(b.Bytes())
}
