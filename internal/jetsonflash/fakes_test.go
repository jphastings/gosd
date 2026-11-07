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
// every OUT transfer. An empty script reads as a timeout.
type fakeDevice struct {
	in  [][]byte
	out [][]byte
}

func (f *fakeDevice) Write(p []byte, _ time.Duration) error {
	f.out = append(f.out, slices.Clone(p))
	return nil
}

func (f *fakeDevice) Read(p []byte, _ time.Duration) (int, error) {
	if len(f.in) == 0 {
		return 0, errTimeout
	}
	n := copy(p, f.in[0])
	f.in = f.in[1:]
	return n, nil
}

func (f *fakeDevice) Close() error { return nil }

func devicePacket(typ, seq uint32, fields ...[]byte) []byte {
	return bytes.Join(frame(typ, seq, fields...), nil)
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
