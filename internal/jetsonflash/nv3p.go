package jetsonflash

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"slices"
	"time"
)

// nv3p is NVIDIA's packet protocol between a host and the recovery applet,
// and later CBoot's flashing server. T210 speaks a variant of the one NVIDIA's
// open-source tegrarcm documents for earlier chips; the constants here are
// what the bench established for T210 (bean gosd-32dw).
const (
	// T210's applet NACKs tegrarcm's version 1 and speaks version 2.
	nv3pVersion = 2

	packetCmd  = 1
	packetData = 2
	packetAck  = 4
	packetNack = 5

	// cmdVersion asks for the running firmware's version; the applet and
	// CBoot's flashing server both answer it.
	cmdVersion = 0x01
	// cmdDownload loads an image into the applet (T210; tegrarcm's DL_BCT
	// and DL_BL were 0x04 and 0x06).
	cmdDownload = 0x03
	// dataChunk is the most one DATA packet carries.
	dataChunk = 1 << 20
	// cmdStatus is how T210's applet reports an outcome, with args
	// {u32 code; char msg[32]}. tegrarcm's STATUS was 0x0a, message first.
	cmdStatus = 0x07
	// cmdWritePartition is the same command number as cmdDownload: CBoot's
	// flashing server takes {char name[40]; u64 size} and then DATA.
	cmdWritePartition = 0x03
	// cmdOEM runs CBoot's OEM commands. Only ever send it oemStorageQuery:
	// OEM type 9 permanently provisions the eMMC's RPMB key.
	cmdOEM = 0x05
	// cmdColdBoot takes u32 args {0}; the device drops off USB as it resets.
	cmdColdBoot = 0x06
	// cmdFlush makes CBoot's flashing server flush written data to storage.
	cmdFlush = 0x0a
	// cmdBoot takes a u32 boot type.
	cmdBoot = 0x09
	// bootFlashingServer is the boot type that brings up CBoot's flashing
	// server.
	bootFlashingServer = 0

	// maxTransfer is the largest single bulk transfer: NVIDIA's T210 tool
	// writes nv3p data 16 KiB at a time, and older kernels refuse more.
	maxTransfer = 16384
	// maxDeviceField bounds a length the device asks us to buffer.
	maxDeviceField = 1 << 24

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
		if code, msg, ok := p.status(); ok {
			return fmt.Sprintf("STATUS seq %d: code %#x %q", p.seq, code, msg)
		}
		return fmt.Sprintf("CMD %#x seq %d, %d-byte args: %s", p.cmd, p.seq, len(p.body), hexPrefix(p.body, 64))
	case packetData:
		return fmt.Sprintf("DATA seq %d, %d bytes: %s", p.seq, len(p.body), hexPrefix(p.body, 64))
	}
	return fmt.Sprintf("packet type %d seq %d", p.typ, p.seq)
}

func (p packet) status() (code uint32, msg string, ok bool) {
	if p.typ != packetCmd || p.cmd != cmdStatus || len(p.body) < 4 {
		return 0, "", false
	}
	// CBoot follows an empty message with uninitialised memory.
	text, _, _ := bytes.Cut(p.body[4:], []byte{0})
	return le32(p.body), string(text), true
}

// frame splits one packet into the bulk transfers NVIDIA's T210 tool uses:
// the header, the body's length (CMD and DATA only), the body in pieces of
// at most maxTransfer, then the checksum. The checksum makes the byte sum of
// the header and body zero; on T210 it leaves the length out.
func frame(typ, seq uint32, body []byte) [][]byte {
	hdr := le32s(nv3pVersion, typ, seq)
	transfers := [][]byte{hdr}
	if typ == packetCmd || typ == packetData {
		transfers = append(transfers, le32s(uint32(len(body))))
	}
	transfers = slices.AppendSeq(transfers, slices.Chunk(body, maxTransfer))
	return append(transfers, le32s(-(byteSum(hdr) + byteSum(body))))
}

// session is one nv3p conversation. Like NVIDIA's tools, each one starts
// its sequence numbers from zero.
type session struct {
	c        conn
	log      io.Writer
	seq      uint32
	pending  []byte       // bytes read from the device but not yet parsed
	onData   func([]byte) // called with each DATA payload drain receives
	statuses int          // how many STATUS packets drain has received
}

func (s *session) send(typ, seq uint32, body []byte) error {
	for _, t := range frame(typ, seq, body) {
		if err := s.c.Write(t, replyTimeout); err != nil {
			if errors.Is(err, errTimeout) {
				// A device that stops reading may have something to say
				// first; fetch it so the trace shows it.
				for s.fill(quietWait) == nil {
				}
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
	if err := s.send(packetCmd, seq, append(le32s(cmd), args...)); err != nil {
		return err
	}
	return s.expectAck(seq)
}

// data sends b in DATA packets of at most dataChunk bytes, waiting for each
// to be acknowledged.
func (s *session) data(b []byte) error {
	for chunk := range slices.Chunk(b, dataChunk) {
		seq := s.seq
		s.seq++
		s.logf("→ DATA seq %d, %d bytes", seq, len(chunk))
		if err := s.send(packetData, seq, chunk); err != nil {
			return err
		}
		if err := s.expectAck(seq); err != nil {
			return err
		}
	}
	return nil
}

// stream sends the rest of data in DATA packets of at most dataChunk
// bytes, so a partition image needn't fit in the BMC's memory.
func (s *session) stream(data io.Reader) error {
	buf := make([]byte, dataChunk)
	for {
		n, err := io.ReadFull(data, buf)
		if n > 0 {
			if err := s.data(buf[:n]); err != nil {
				return err
			}
		}
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return nil
		} else if err != nil {
			return err
		}
	}
}

func (s *session) expectAck(seq uint32) error {
	p, err := s.read(replyTimeout)
	if err != nil {
		return err
	}
	switch {
	case p.typ == packetNack:
		// The applet follows a NACK with a report saying why; log it.
		_ = s.drain(quietWait)
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
			if p.typ == packetData && s.onData != nil {
				s.onData(p.body)
			}
			if err := s.send(packetAck, p.seq, nil); err != nil {
				return err
			}
			if _, _, isStatus := p.status(); isStatus {
				if err := s.send(packetAck, p.seq, nil); err != nil {
					return err
				}
			}
		}
		if code, msg, ok := p.status(); ok {
			s.statuses++
			if code != 0 {
				return fmt.Errorf("the device reported status %#x: %s", code, msg)
			}
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
		sum -= byteSum(l)
		n := le32(l)
		if n > maxDeviceField {
			return p, fmt.Errorf("the device announced a %d-byte field, which is implausible", n)
		}
		if p.typ == packetCmd && n < 4 {
			return p, fmt.Errorf("the device sent a CMD too short to hold a command: % x", l)
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
