package jetsonflash

import (
	"errors"
	"fmt"
	"io"
	"time"
)

// conn is one claimed Tegra USB recovery-mode device. Every call is exactly
// one bulk transfer, on endpoint 0x01 (OUT) or 0x81 (IN): the Tegra side
// posts one USB read per protocol field, so transfer boundaries matter.
type conn interface {
	Write(p []byte, timeout time.Duration) error
	Read(p []byte, timeout time.Duration) (int, error)
	Close() error
}

var errTimeout = errors.New("USB transfer timed out")

// tracedConn logs every transfer, timestamped from start, so a bench run
// records exactly what the device answered and how long each gap was.
type tracedConn struct {
	conn
	w     io.Writer
	start time.Time
}

func (t tracedConn) Write(p []byte, timeout time.Duration) error {
	err := t.conn.Write(p, timeout)
	t.trace("OUT", p, err)
	return err
}

func (t tracedConn) Read(p []byte, timeout time.Duration) (int, error) {
	n, err := t.conn.Read(p, timeout)
	t.trace("IN ", p[:n], err)
	return n, err
}

func (t tracedConn) trace(dir string, p []byte, err error) {
	line := fmt.Sprintf("%9.3fs %s %6d  %s", time.Since(t.start).Seconds(), dir, len(p), hexPrefix(p, 64))
	if err != nil {
		line += "  error: " + err.Error()
	}
	_, _ = fmt.Fprintln(t.w, line)
}

func hexPrefix(p []byte, limit int) string {
	if len(p) > limit {
		return fmt.Sprintf("% x …", p[:limit])
	}
	return fmt.Sprintf("% x", p)
}
