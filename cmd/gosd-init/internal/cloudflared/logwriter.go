package cloudflared

import (
	"bytes"
	"sync"
)

// maxBufferedLine bounds how many bytes a lineWriter accumulates before
// flushing a line that hasn't seen a newline yet: cloudflared emitting
// pathological output with no line breaks at all must never grow PID 1's
// memory without bound.
const maxBufferedLine = 4096

// lineWriter is an io.Writer that splits everything written to it on '\n'
// and logs each complete line, prefixed "cloudflared: ", through the
// injected log func. Nothing in the standard library does this
// line-splitting-with-a-prefix job, so supervise (cloudflared.go) creates
// two independent instances — one for cloudflared's stdout, one for its
// stderr — each keeping its own buffer, so a partial line on one stream is
// never interleaved mid-line with the other's output.
//
// Its buffer is mutex-guarded because Write and Close are genuinely called
// concurrently in production: StartProcess never calls cmd.Wait (see its
// doc comment — that's the reaper's job), so os/exec's own goroutine that
// copies the child's stdout/stderr pipe into this writer is not guaranteed
// to have drained by the time Deps.Wait(pid) returns and runOnce calls
// Close. A stray Write arriving just after Close simply starts a fresh
// buffer that never gets flushed again — a possible lost trailing partial
// line, harmless for what is, after all, best-effort diagnostic logging —
// but without the mutex it would instead be a genuine data race on buf.
type lineWriter struct {
	log func(format string, args ...any)

	mu  sync.Mutex
	buf []byte
}

func newLineWriter(log func(format string, args ...any)) *lineWriter {
	return &lineWriter{log: log}
}

// Write implements io.Writer, buffering p and flushing one log line for
// every '\n' found — a single Write spanning several lines, or one line
// arriving split across several Writes, are both handled the same way. Any
// remainder left over with no newline in sight is flushed early (with a
// truncation note) once it exceeds maxBufferedLine, rather than growing the
// buffer without limit.
func (w *lineWriter) Write(p []byte) (int, error) {
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
	if len(w.buf) > maxBufferedLine {
		w.flushLocked(true)
	}
	return total, nil
}

// flushLocked logs the accumulated buffer as a single line and clears it.
// truncated notes that the line is being cut off for exceeding
// maxBufferedLine, rather than ending on its own newline. Callers must hold
// w.mu.
func (w *lineWriter) flushLocked(truncated bool) {
	if truncated {
		w.log("cloudflared: %s [line truncated at %d bytes]", w.buf, maxBufferedLine)
	} else {
		w.log("cloudflared: %s", w.buf)
	}
	w.buf = w.buf[:0]
}

// Close flushes any trailing partial line still buffered (no newline before
// the underlying pipe closed), so the last line cloudflared writes before
// exiting mid-line isn't silently dropped — except in the race Close itself
// documents (see the type doc comment): a Write landing after this Close
// has already run starts a buffer nothing will flush again.
func (w *lineWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.buf) > 0 {
		w.flushLocked(false)
	}
	return nil
}
