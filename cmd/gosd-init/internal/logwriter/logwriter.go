// Package logwriter implements a line-splitting, prefixed log writer:
// buffers everything written to it, splits it on '\n', and logs each
// complete line through an injected log func with a fixed prefix. Extracted
// from cmd/gosd-init/internal/cloudflared (bean gosd-wxjy) into a shared
// package so a second gosd-init-supervised agent (epic gosd-65uy) can reuse
// the exact line-splitting/overflow behavior with its own prefix, instead of
// duplicating it.
package logwriter

import (
	"bytes"
	"sync"
)

// MaxBufferedLine bounds how many bytes a Writer accumulates before
// flushing a line that hasn't seen a newline yet: a supervised child
// emitting pathological output with no line breaks at all must never grow
// PID 1's memory without bound.
const MaxBufferedLine = 4096

// Writer is an io.Writer that splits everything written to it on '\n' and
// logs each complete line, prefixed with the string given to New, through
// the injected log func. Nothing in the standard library does this
// line-splitting-with-a-prefix job, so a supervisor creates two independent
// instances — one for a child's stdout, one for its stderr — each keeping
// its own buffer, so a partial line on one stream is never interleaved
// mid-line with the other's output.
//
// Its buffer is mutex-guarded because Write and Close are genuinely called
// concurrently in production: a supervisor never calls the child process's
// own Wait itself (that's a central PID-1 reaper's job — see
// cloudflared.Deps.Wait's doc comment), so os/exec's own goroutine that
// copies the child's stdout/stderr pipe into this writer is not guaranteed
// to have drained by the time the reaper's wait returns and the supervisor
// calls Close. A stray Write arriving just after Close simply starts a
// fresh buffer that never gets flushed again — a possible lost trailing
// partial line, harmless for what is, after all, best-effort diagnostic
// logging — but without the mutex it would instead be a genuine data race
// on buf.
type Writer struct {
	prefix string
	log    func(format string, args ...any)

	mu  sync.Mutex
	buf []byte
}

// New creates a Writer that logs each complete line through log, prefixed
// with prefix (e.g. "cloudflared: ").
func New(prefix string, log func(format string, args ...any)) *Writer {
	return &Writer{prefix: prefix, log: log}
}

// Write implements io.Writer, buffering p and flushing one log line for
// every '\n' found — a single Write spanning several lines, or one line
// arriving split across several Writes, are both handled the same way. Any
// remainder left over with no newline in sight is flushed early (with a
// truncation note) once it exceeds MaxBufferedLine, rather than growing the
// buffer without limit.
func (w *Writer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	total := len(p)
	for {
		idx := bytes.IndexByte(p, '\n')
		if idx < 0 {
			break
		}
		w.buf = append(w.buf, p[:idx]...)
		w.flushLocked(false)
		p = p[idx+1:]
	}
	w.buf = append(w.buf, p...)
	if len(w.buf) > MaxBufferedLine {
		w.flushLocked(true)
	}
	return total, nil
}

// flushLocked logs the accumulated buffer as a single line and clears it.
// truncated notes that the line is being cut off for exceeding
// MaxBufferedLine, rather than ending on its own newline. Callers must hold
// w.mu.
func (w *Writer) flushLocked(truncated bool) {
	if truncated {
		w.log("%s%s [line truncated at %d bytes]", w.prefix, w.buf, MaxBufferedLine)
	} else {
		w.log("%s%s", w.prefix, w.buf)
	}
	w.buf = w.buf[:0]
}

// Close flushes any trailing partial line still buffered (no newline before
// the underlying pipe closed), so the last line a child writes before
// exiting mid-line isn't silently dropped — except in the race Close itself
// documents (see the type doc comment): a Write landing after this Close
// has already run starts a buffer nothing will flush again.
func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.buf) > 0 {
		w.flushLocked(false)
	}
	return nil
}
