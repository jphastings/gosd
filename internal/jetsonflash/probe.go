package jetsonflash

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

// ProbeOptions tunes RunProbe.
type ProbeOptions struct {
	Wait     time.Duration // first-reply wait for each command
	DataFile string        // sent as DATA after each command's first reply
	OutFile  string        // receives every DATA payload the device sends
}

type probeCmd struct {
	cmd  uint32
	args []byte
}

func (p probeCmd) String() string { return strings.TrimSpace(fmt.Sprintf("%#x %x", p.cmd, p.args)) }

// RunProbe sends each spec (CMD, CMD:HEXARGS or a FIRST-LAST range) to a
// CBoot flashing server that is already running, one session per command,
// logging every transfer to log. A command the device rejects is logged and
// skipped; losing the device or its version reply stops the probe.
func RunProbe(specs []string, opts ProbeOptions, log io.Writer) (err error) {
	cmds, err := parseProbeSpecs(specs)
	if err != nil {
		return err
	}
	var data []byte
	if opts.DataFile != "" {
		if data, err = os.ReadFile(opts.DataFile); err != nil {
			return err
		}
	}
	var out io.Writer
	if opts.OutFile != "" {
		f, err := os.Create(opts.OutFile)
		if err != nil {
			return err
		}
		defer func() { err = errors.Join(err, f.Close()) }()
		out = f
	}
	start := time.Now()
	open := func() (conn, error) {
		c, err := openRecovery(deviceWait)
		if err != nil {
			return nil, err
		}
		return tracedConn{conn: c, w: log, start: start}, nil
	}
	return probe(open, cmds, data, opts.Wait, out, log)
}

func parseProbeSpecs(specs []string) ([]probeCmd, error) {
	var cmds []probeCmd
	for _, spec := range specs {
		name, hexArgs, hasArgs := strings.Cut(spec, ":")
		first, last, isRange := strings.Cut(name, "-")
		lo, err := strconv.ParseUint(first, 0, 32)
		hi := lo
		if err == nil && isRange {
			if hasArgs {
				err = errors.New("a range takes no args")
			} else if hi, err = strconv.ParseUint(last, 0, 32); err == nil && hi < lo {
				err = errors.New("the range runs backwards")
			}
		}
		var args []byte
		if err == nil && hasArgs {
			args, err = hex.DecodeString(hexArgs)
		}
		if err != nil {
			return nil, fmt.Errorf("bad probe spec %q (want CMD, CMD:HEXARGS or FIRST-LAST): %w", spec, err)
		}
		for c := lo; c <= hi; c++ {
			cmds = append(cmds, probeCmd{uint32(c), args})
		}
	}
	return cmds, nil
}

func probe(open func() (conn, error), cmds []probeCmd, data []byte, wait time.Duration, out, log io.Writer) error {
	for _, pc := range cmds {
		_, _ = fmt.Fprintf(log, "== %v\n", pc)
		c, err := open()
		if err != nil {
			return fmt.Errorf("opening the device to send %v (did the module reset?): %w", pc, err)
		}
		cmdErr, fatal := probeOne(c, pc, data, wait, out, log)
		if err := errors.Join(fatal, c.Close()); err != nil {
			return fmt.Errorf("%v: %w", pc, err)
		}
		if cmdErr != nil {
			_, _ = fmt.Fprintf(log, "command %v failed: %v\n", pc, cmdErr)
		}
	}
	return nil
}

// probeOne returns the command's own failure separately from the failure of
// the version query that opens every session, which is fatal to the probe.
func probeOne(c conn, pc probeCmd, data []byte, wait time.Duration, out, log io.Writer) (cmdErr, fatal error) {
	s := &session{c: c, log: log}
	if err := s.version(); err != nil {
		return nil, fmt.Errorf("version query: %w", err)
	}
	if out != nil {
		s.onData = func(b []byte) { _, _ = out.Write(b) }
	}
	if cmdErr = s.command(pc.cmd, pc.args); cmdErr != nil {
		return cmdErr, nil
	}
	if cmdErr = s.drain(wait); cmdErr != nil || data == nil {
		return cmdErr, nil
	}
	if cmdErr = s.data(data); cmdErr != nil {
		return cmdErr, nil
	}
	return s.drain(wait), nil
}
