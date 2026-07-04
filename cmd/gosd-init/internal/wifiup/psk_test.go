package wifiup

import (
	"encoding/hex"
	"strings"
	"testing"
)

// TestDerivePSKMatchesIEEETestVectors checks DerivePSK against the
// well-known IEEE 802.11i-2004 Annex H.4 PBKDF2-SHA1 WPA test vectors.
func TestDerivePSKMatchesIEEETestVectors(t *testing.T) {
	cases := []struct {
		name       string
		passphrase string
		ssid       string
		want       string
	}{
		{
			name:       "IEEE vector 1",
			passphrase: "password",
			ssid:       "IEEE",
			want:       "f42c6fc52df0ebef9ebb4b90b38a5f902e83fe1b135a70e23aed762e9710a12e",
		},
		{
			name:       "IEEE vector 2",
			passphrase: "ThisIsAPassword",
			ssid:       "ThisIsASSID",
			want:       "0dc0d6eb90555ed6419756b9a15ec3e3209b63df707dd508d14581f8982721af",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// The published vectors are conventionally quoted as 65 hex
			// characters (an errant trailing nibble in some transcriptions);
			// only the first 64 (32 bytes) are the actual PSK.
			want := tc.want[:64]

			psk, err := DerivePSK(tc.passphrase, tc.ssid)
			if err != nil {
				t.Fatalf("DerivePSK() error = %v", err)
			}
			got := hex.EncodeToString(psk[:])
			if !strings.EqualFold(got, want) {
				t.Errorf("DerivePSK(%q, %q) = %s, want %s", tc.passphrase, tc.ssid, got, want)
			}
		})
	}
}

func TestParsePSKHexRoundTripsWithDerivePSK(t *testing.T) {
	derived, err := DerivePSK("correcthorsebatterystaple", "gosd-test")
	if err != nil {
		t.Fatalf("DerivePSK() error = %v", err)
	}

	parsed, err := ParsePSKHex(hex.EncodeToString(derived[:]))
	if err != nil {
		t.Fatalf("ParsePSKHex() error = %v", err)
	}
	if parsed != derived {
		t.Errorf("ParsePSKHex(hex(derived)) = %x, want %x", parsed, derived)
	}
}

func TestParsePSKHexRejectsWrongLength(t *testing.T) {
	if _, err := ParsePSKHex("deadbeef"); err == nil {
		t.Error("ParsePSKHex(short hex) = nil error, want an error")
	}
}

func TestParsePSKHexRejectsNonHex(t *testing.T) {
	notHex := strings.Repeat("z", 64)
	if _, err := ParsePSKHex(notHex); err == nil {
		t.Error("ParsePSKHex(non-hex) = nil error, want an error")
	}
}

func TestIsHexPSK(t *testing.T) {
	cases := map[string]bool{
		strings.Repeat("a", 64): true,
		strings.Repeat("A", 64): true,
		"a password":            false,
		strings.Repeat("a", 63): false,
		strings.Repeat("z", 64): false, // right length, not valid hex
		"":                      false,
	}
	for s, want := range cases {
		if got := isHexPSK(s); got != want {
			t.Errorf("isHexPSK(%q) = %v, want %v", s, got, want)
		}
	}
}
