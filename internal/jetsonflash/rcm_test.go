package jetsonflash

import (
	"bytes"
	"slices"
	"strings"
	"testing"
)

func TestBootROMSendsBothMessagesIn4KiBChunksAndReturnsTheUID(t *testing.T) {
	uid := []byte("0123456789abcdef")
	dev := &fakeDevice{in: [][]byte{uid, le32s(rcmVersion), le32s(0)}}
	rcm0, rcm1 := make([]byte, 1032), make([]byte, 9000)

	got, err := bootROM(dev, rcm0, rcm1)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(got, uid) {
		t.Errorf("UID %q, want %q", got, uid)
	}
	if got, want := sizes(dev.out), []int{1032, 4096, 4096, 808}; !slices.Equal(got, want) {
		t.Errorf("transfer sizes %v, want %v", got, want)
	}
}

func TestBootROMFailuresSayWhatHappened(t *testing.T) {
	uid := make([]byte, 16)
	for name, tc := range map[string]struct {
		in   [][]byte
		want string
	}{
		"no UID":           {nil, "power-cycle"},
		"applet rejected":  {[][]byte{uid, le32s(rcmVersion), le32s(0x13)}, "recovery applet download with 0x00000013"},
		"unexpected chip":  {[][]byte{uid, le32s(0x00400001)}, "RCM version query with 0x00400001"},
		"no answer to RCM": {[][]byte{uid}, "answer to the RCM version query"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := bootROM(&fakeDevice{in: tc.in}, make([]byte, 1032), make([]byte, 1032))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("got %v, want an error mentioning %q", err, tc.want)
			}
		})
	}
}
