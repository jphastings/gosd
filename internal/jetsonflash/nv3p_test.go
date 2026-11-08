package jetsonflash

import (
	"bytes"
	"io"
	"slices"
	"strings"
	"testing"
)

// The device posts one USB read per field, so the transfer boundaries are
// part of the protocol.
func TestCommandGoesOutOneFieldPerTransfer(t *testing.T) {
	dev := &fakeDevice{in: [][]byte{unhex(t, "02000000 04000000 00000000 faffffff")}}
	s := &session{c: dev, log: io.Discard}

	if err := s.command(cmdVersion, nil); err != nil {
		t.Fatal(err)
	}

	want := [][]byte{
		unhex(t, "02000000 01000000 00000000"),
		unhex(t, "04000000"),
		unhex(t, "01000000"),
		unhex(t, "fcffffff"),
	}
	if !slices.EqualFunc(dev.out, want, bytes.Equal) {
		t.Errorf("sent % x, want % x", dev.out, want)
	}
}

func TestDataGoesOutIn16KiBWrites(t *testing.T) {
	dev := &fakeDevice{in: [][]byte{devicePacket(packetAck, 0, nil)}}
	s := &session{c: dev, log: io.Discard}

	if err := s.data(bytes.Repeat([]byte{0xa5}, 40000)); err != nil {
		t.Fatal(err)
	}

	if got, want := sizes(dev.out), []int{12, 4, 16384, 16384, 7232, 4}; !slices.Equal(got, want) {
		t.Errorf("transfer sizes %v, want %v", got, want)
	}
}

func TestDrainAcksEachPacketAndStatusTwice(t *testing.T) {
	info := devicePacket(packetData, 7, make([]byte, 72))
	status := devicePacket(packetCmd, 8, append(le32s(cmdStatus, 0), make([]byte, 32)...))
	dev := &fakeDevice{in: [][]byte{append(info, status...)}}
	s := &session{c: dev, log: io.Discard}

	if err := s.drain(quietWait); err != nil {
		t.Fatal(err)
	}

	// The applet takes a second ACK after a STATUS; with only one, it
	// swallows the next command (bench, 2026-11-08).
	want := slices.Concat(frame(packetAck, 7, nil), frame(packetAck, 8, nil), frame(packetAck, 8, nil))
	if !slices.EqualFunc(dev.out, want, bytes.Equal) {
		t.Errorf("sent % x, want an ACK for sequence 7 and two for 8", dev.out)
	}
}

// The NACK and STATUS replies are bytes the T210 applet sent on the bench.
func TestDeviceRefusalsAreErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		reply string
		want  string
	}{
		"NACK": {"02000000 05000000 00000000 03000000 f6ffffff", "bad data"},
		"failed STATUS": {"02000000 01000000 00000000 28000000 07000000 00001200" +
			"556e77616e746564206572726f7220696e20337020736572766572 0000000000 e3f5ffff", "Unwanted error in 3p server"},
		"corrupt checksum": {"02000000 04000000 00000000 fbffffff", "checksum"},
	} {
		t.Run(name, func(t *testing.T) {
			s := &session{c: &fakeDevice{in: [][]byte{unhex(t, tc.reply)}}, log: io.Discard}
			if err := s.drain(quietWait); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("got %v, want an error mentioning %q", err, tc.want)
			}
		})
	}
}
