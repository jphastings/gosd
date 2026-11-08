package jetsonflash

import (
	"bytes"
	"encoding/hex"
	"slices"
	"strings"
	"testing"
	"time"
)

// fakeDevice plays back scripted IN transfers, one per Read, and records
// every OUT transfer. A nil entry, like an empty script, reads as a timeout.
type fakeDevice struct {
	in  [][]byte
	out [][]byte
	// gone, once the script runs out, is what Read fails with instead of
	// timing out.
	gone error
}

func (f *fakeDevice) Write(p []byte, _ time.Duration) error {
	f.out = append(f.out, slices.Clone(p))
	return nil
}

func (f *fakeDevice) Read(p []byte, _ time.Duration) (int, error) {
	if len(f.in) == 0 {
		if f.gone != nil {
			return 0, f.gone
		}
		return 0, errTimeout
	}
	in := f.in[0]
	f.in = f.in[1:]
	if in == nil {
		return 0, errTimeout
	}
	return copy(p, in), nil
}

func (f *fakeDevice) Close() error { return nil }

func devicePacket(typ, seq uint32, body []byte) []byte {
	return bytes.Join(frame(typ, seq, body), nil)
}

func unhex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(strings.ReplaceAll(s, " ", ""))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func sizes(transfers [][]byte) []int {
	var n []int
	for _, t := range transfers {
		n = append(n, len(t))
	}
	return n
}

// versionReply is what the applet and CBoot send back to a version query.
func versionReply() [][]byte {
	status := devicePacket(packetCmd, 3, le32s(cmdStatus, 0))
	reply := slices.Concat(devicePacket(packetCmd, 1, le32s(0x08, 16)), devicePacket(packetData, 2, make([]byte, 16)), status)
	return [][]byte{devicePacket(packetAck, 0, nil), reply, nil}
}

// download is the device's side of one image download or partition write
// of one DATA packet: the empty CMD 0x08 after the command, the ACK of the
// DATA, then the STATUS.
func download(seq uint32) [][]byte {
	return [][]byte{
		devicePacket(packetAck, seq, nil), devicePacket(packetCmd, 9, le32s(0x08, 0)), nil,
		devicePacket(packetAck, seq+1, nil), devicePacket(packetCmd, 9, le32s(cmdStatus, 0)), nil,
	}
}
