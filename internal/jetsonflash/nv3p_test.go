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

	if err := s.command(cmdGetPlatformInfo, nil); err != nil {
		t.Fatal(err)
	}

	want := [][]byte{
		unhex(t, "02000000 01000000 00000000"),
		unhex(t, "00000000"),
		unhex(t, "01000000"),
		unhex(t, "fcffffff"),
	}
	if !slices.EqualFunc(dev.out, want, bytes.Equal) {
		t.Errorf("sent % x, want % x", dev.out, want)
	}
}

func TestDataGoesOutIn16KiBWritesWithAValidChecksum(t *testing.T) {
	dev := &fakeDevice{in: [][]byte{devicePacket(packetAck, 0)}}
	s := &session{c: dev, log: io.Discard}

	if err := s.data(bytes.Repeat([]byte{0xa5}, 40000)); err != nil {
		t.Fatal(err)
	}

	if got, want := sizes(dev.out), []int{12, 4, 16384, 16384, 7232, 4}; !slices.Equal(got, want) {
		t.Errorf("transfer sizes %v, want %v", got, want)
	}
	if sum := byteSum(bytes.Join(dev.out[:len(dev.out)-1], nil)) + le32(dev.out[len(dev.out)-1]); sum != 0 {
		t.Errorf("packet doesn't checksum to zero: %#x", sum)
	}
}

func TestDrainAcksEachPacketEvenWhenTheyArriveTogether(t *testing.T) {
	info := devicePacket(packetData, 7, le32s(72), make([]byte, 72))
	status := devicePacket(packetCmd, 8, le32s(4), le32s(cmdStatus, 0))
	dev := &fakeDevice{in: [][]byte{append(info, status...)}}
	s := &session{c: dev, log: io.Discard}

	if err := s.drain(quietWait); err != nil {
		t.Fatal(err)
	}

	want := append(frame(packetAck, 7), frame(packetAck, 8)...)
	if !slices.EqualFunc(dev.out, want, bytes.Equal) {
		t.Errorf("sent % x, want ACKs for sequences 7 and 8", dev.out)
	}
}

func TestDeviceRefusalsAreErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		reply []byte
		want  string
	}{
		"NACK":             {devicePacket(packetNack, 0, le32s(2)), "bad command"},
		"failed STATUS":    {devicePacket(packetCmd, 0, le32s(4), le32s(cmdStatus, 9)), "status 9"},
		"corrupt checksum": {unhex(t, "02000000 04000000 00000000 fbffffff"), "checksum"},
	} {
		t.Run(name, func(t *testing.T) {
			s := &session{c: &fakeDevice{in: [][]byte{tc.reply}}, log: io.Discard}
			if err := s.drain(quietWait); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("got %v, want an error mentioning %q", err, tc.want)
			}
		})
	}
}
