package jetsonflash

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"slices"
	"time"
)

// nv3p is NVIDIA's packet protocol between a host and the recovery applet,
// and later CBoot's flashing server. The packet layout, checksum rule,
// command numbers and NACK codes are facts from NVIDIA's open-source
// tegrarcm (BSD-3-Clause, src/nv3p.h and src/nv3p.c), which documents the
// T20-T124 generation; whether T210 kept them is what bean gosd-32dw's
// spike is testing.
const (
	// T210's applet NACKs tegrarcm's version 1 and speaks version 2.
	nv3pVersion = 2

	packetCmd  = 1
	packetData = 2
	packetAck  = 4
	packetNack = 5

	cmdGetPlatformInfo = 0x01
	cmdDownloadBCT     = 0x04
	cmdDownloadBL      = 0x06
	cmdStatus          = 0x0a

	// maxTransfer is the largest single bulk transfer: NVIDIA's T210 tool
	// writes nv3p data 16 KiB at a time, and older kernels refuse more.
	maxTransfer = 16384
	// maxDeviceField bounds a length the device asks us to buffer.
	maxDeviceField = 1 << 16

	replyTimeout = 5 * time.Second
	// quietWait is how long the device must stay silent before we take it
	// that nothing more is coming and it is waiting for the host. Keep it
	// short: the applet and CBoot only wait briefly between steps.
	quietWait = 250 * time.Millisecond
)

var nackReasons = map[uint32]string{1: "success", 2: "bad command", 3: "bad data"}

type packet struct {
	typ, seq uint32
	cmd      uint32 // CMD packets only
	body     []byte // CMD args, DATA payload or NACK code
}

func (p packet) String() string {
	switch p.typ {
	case packetAck:
		return fmt.Sprintf("ACK seq %d", p.seq)
	case packetNack:
		code := le32(p.body)
		return fmt.Sprintf("NACK seq %d: %d (%s)", p.seq, code, nackReasons[code])
	case packetCmd:
		return fmt.Sprintf("CMD %#x seq %d, %d-byte args: %s", p.cmd, p.seq, len(p.body), hexPrefix(p.body, 64))
	case packetData:
		return fmt.Sprintf("DATA seq %d, %d bytes: %s", p.seq, len(p.body), hexPrefix(p.body, 64))
	}
	return fmt.Sprintf("packet type %d seq %d", p.typ, p.seq)
}

// status is the code in a device STATUS command. tegrarcm's STATUS args are
// {char msg[32]; u32 code; u32 flags}; a 4-byte form is taken to be the
// code alone.
func (p packet) status() (uint32, bool) {
	if p.typ != packetCmd || p.cmd != cmdStatus {
		return 0, false
	}
	switch len(p.body) {
	case 40:
		return le32(p.body[32:]), true
	case 4:
		return le32(p.body), true
	}
	return 0, false
}

// frame splits one packet into the bulk transfers NVIDIA's T210 tool uses:
// the header, then each field (in pieces of at most maxTransfer), then the
// checksum. The checksum makes the byte sum of the whole packet zero.
func frame(typ, seq uint32, fields ...[]byte) [][]byte {
	hdr := le32s(nv3pVersion, typ, seq)
	sum := byteSum(hdr)
	transfers := [][]byte{hdr}
	for _, f := range fields {
		sum += byteSum(f)
		transfers = slices.AppendSeq(transfers, slices.Chunk(f, maxTransfer))
	}
	return append(transfers, le32s(-sum))
}

// session is one nv3p conversation. Like NVIDIA's tools, each one starts
// its sequence numbers from zero.
type session struct {
	c       conn
	log     io.Writer
	seq     uint32
	pending []byte // bytes read from the device but not yet parsed
}

func (s *session) send(typ, seq uint32, fields ...[]byte) error {
	for _, t := range frame(typ, seq, fields...) {
		if err := s.c.Write(t, replyTimeout); err != nil {
			if errors.Is(err, errTimeout) {
				// A device that stops reading may have something to say
				// first; fetch it so the trace shows it.
				_ = s.fill(quietWait)
			}
			return err
		}
	}
	return nil
}

// command sends a CMD and waits for the device to acknowledge it.
func (s *session) command(cmd uint32, args []byte) error {
	seq := s.seq
	s.seq++
	s.logf("→ CMD %#x seq %d, %d-byte args: % x", cmd, seq, len(args), args)
	if err := s.send(packetCmd, seq, le32s(uint32(len(args))), append(le32s(cmd), args...)); err != nil {
		return err
	}
	return s.expectAck(seq)
}

// data sends one DATA packet carrying all of b and waits for the
// acknowledgement. NVIDIA's T210 tool sends a whole file as one packet.
func (s *session) data(b []byte) error {
	seq := s.seq
	s.seq++
	s.logf("→ DATA seq %d, %d bytes", seq, len(b))
	if err := s.send(packetData, seq, le32s(uint32(len(b))), b); err != nil {
		return err
	}
	return s.expectAck(seq)
}

func (s *session) expectAck(seq uint32) error {
	p, err := s.read(replyTimeout)
	if err != nil {
		return err
	}
	switch {
	case p.typ == packetNack:
		return fmt.Errorf("the device refused it: %v", p)
	case p.typ != packetAck:
		return fmt.Errorf("expected an ACK, got %v", p)
	case p.seq != seq:
		return fmt.Errorf("the ACK was for sequence %d, want %d", p.seq, seq)
	}
	return nil
}

// drain acknowledges whatever the device sends until it falls quiet,
// failing on a NACK or a non-zero STATUS. It waits up to first for the
// first packet, and quietWait for each one after.
func (s *session) drain(first time.Duration) error {
	for wait := first; ; wait = quietWait {
		if len(s.pending) == 0 {
			if err := s.fill(wait); errors.Is(err, errTimeout) {
				return nil
			} else if err != nil {
				return err
			}
		}
		p, err := s.read(replyTimeout)
		if err != nil {
			return err
		}
		switch p.typ {
		case packetNack:
			return fmt.Errorf("the device refused it: %v", p)
		case packetCmd, packetData:
			if err := s.send(packetAck, p.seq); err != nil {
				return err
			}
		}
		if code, ok := p.status(); ok && code != 0 {
			return fmt.Errorf("the device reported status %d", code)
		}
	}
}

func (s *session) read(timeout time.Duration) (packet, error) {
	var sum uint32
	take := func(n int) ([]byte, error) {
		for len(s.pending) < n {
			if err := s.fill(timeout); err != nil {
				return nil, err
			}
		}
		b := slices.Clone(s.pending[:n])
		s.pending = s.pending[n:]
		sum += byteSum(b)
		return b, nil
	}

	hdr, err := take(12)
	if err != nil {
		return packet{}, err
	}
	if v := le32(hdr); v != nv3pVersion {
		return packet{}, fmt.Errorf("not an nv3p packet (version %d): % x", v, hdr)
	}
	p := packet{typ: le32(hdr[4:]), seq: le32(hdr[8:])}
	switch p.typ {
	case packetAck:
	case packetNack:
		p.body, err = take(4)
	case packetCmd, packetData:
		var l []byte
		if l, err = take(4); err != nil {
			break
		}
		n := le32(l)
		if n > maxDeviceField {
			return p, fmt.Errorf("the device announced a %d-byte field, which is implausible", n)
		}
		if p.typ == packetCmd {
			n += 4
		}
		if p.body, err = take(int(n)); err == nil && p.typ == packetCmd {
			p.cmd, p.body = le32(p.body), p.body[4:]
		}
	default:
		return p, fmt.Errorf("unknown nv3p packet type %d: % x", p.typ, hdr)
	}
	if err != nil {
		return p, err
	}
	payloadSum := sum
	ck, err := take(4)
	if err != nil {
		return p, err
	}
	if payloadSum+le32(ck) != 0 {
		return p, fmt.Errorf("bad checksum on %v", p)
	}
	s.logf("← %v", p)
	return p, nil
}

// fill appends one bulk IN transfer to pending.
func (s *session) fill(timeout time.Duration) error {
	buf := make([]byte, maxTransfer)
	n, err := s.c.Read(buf, timeout)
	s.pending = append(s.pending, buf[:n]...)
	return err
}

func (s *session) logf(format string, args ...any) {
	_, _ = fmt.Fprintf(s.log, format+"\n", args...)
}

func le32(b []byte) uint32 { return binary.LittleEndian.Uint32(b) }

func le32s(vs ...uint32) []byte {
	var b []byte
	for _, v := range vs {
		b = binary.LittleEndian.AppendUint32(b, v)
	}
	return b
}

func byteSum(b []byte) uint32 {
	var sum uint32
	for _, c := range b {
		sum += uint32(c)
	}
	return sum
}
