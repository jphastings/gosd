package boot

import (
	"fmt"
	"io"
	"sync"
)

// consolePrefix is prepended to every gosd-init log line, per the locked
// boot sequence.
const consolePrefix = "[gosd] "

// Logger writes prefixed log lines to an underlying writer (in production,
// /dev/console). Printf is called from more than one goroutine — the main
// boot sequence, StartNetworking's own goroutine, and (gosd-42vb) the
// AppSignalsReady poller — so writes are serialized with a mutex; nothing
// about the underlying writer (a raw fd in production, a plain
// *bytes.Buffer in tests) can be assumed safe for concurrent use on its
// own.
type Logger struct {
	mu  sync.Mutex
	out io.Writer
}

// NewLogger wraps w so every line written through Printf is prefixed with
// "[gosd] ".
func NewLogger(w io.Writer) *Logger {
	return &Logger{out: w}
}

// Printf formats and writes a single log line. format should not include a
// trailing newline; Printf adds one. Safe for concurrent use.
func (l *Logger) Printf(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = fmt.Fprintf(l.out, consolePrefix+format+"\n", args...)
}
