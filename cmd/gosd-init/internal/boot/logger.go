package boot

import (
	"fmt"
	"io"
)

// consolePrefix is prepended to every gosd-init log line, per the locked
// boot sequence.
const consolePrefix = "[gosd] "

// Logger writes prefixed log lines to an underlying writer (in production,
// /dev/console).
type Logger struct {
	out io.Writer
}

// NewLogger wraps w so every line written through Printf is prefixed with
// "[gosd] ".
func NewLogger(w io.Writer) *Logger {
	return &Logger{out: w}
}

// Printf formats and writes a single log line. format should not include a
// trailing newline; Printf adds one.
func (l *Logger) Printf(format string, args ...any) {
	_, _ = fmt.Fprintf(l.out, consolePrefix+format+"\n", args...)
}
