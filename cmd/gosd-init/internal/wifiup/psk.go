package wifiup

import (
	"crypto/pbkdf2"
	"crypto/sha1" //nolint:gosec // SHA1 is mandated by the WPA2 PSK derivation standard (IEEE 802.11i-2004 Annex H.4), not a choice.
	"encoding/hex"
	"fmt"
)

// DerivePSK computes the WPA2 pairwise master key for ssid/passphrase per
// IEEE 802.11i-2004 Annex H.4: PBKDF2-HMAC-SHA1(passphrase, ssid, 4096
// iterations, 256 bits). ssid is the raw salt bytes, exactly as
// transmitted over the air — it is not case-folded or otherwise
// normalized, since the standard doesn't either.
//
// Uses the standard library's crypto/pbkdf2 (available since this
// module's Go version), rather than golang.org/x/crypto/pbkdf2, so this
// package pulls in no additional dependency for it.
func DerivePSK(passphrase, ssid string) ([32]byte, error) {
	var psk [32]byte
	key, err := pbkdf2.Key(sha1.New, passphrase, []byte(ssid), 4096, 32)
	if err != nil {
		return psk, fmt.Errorf("deriving PSK for SSID %q: %w", ssid, err)
	}
	copy(psk[:], key)
	return psk, nil
}

// ParsePSKHex decodes a pre-hashed 64-hex-character PSK directly, skipping
// PBKDF2 entirely. This is the form v0.2's Imager provisioning is expected
// to write to config.json — deriving the PSK once at flash time so the
// plaintext passphrase never has to be baked onto the image.
func ParsePSKHex(s string) ([32]byte, error) {
	var psk [32]byte
	if !isHexPSK(s) {
		return psk, fmt.Errorf("PSK must be 64 hex characters (32 bytes), got %d characters", len(s))
	}
	decoded, err := hex.DecodeString(s)
	if err != nil {
		return psk, fmt.Errorf("PSK is not valid hex: %w", err)
	}
	copy(psk[:], decoded)
	return psk, nil
}

// isHexPSK reports whether s looks like a pre-hashed 64-hex-character PSK
// rather than a plaintext passphrase, so ConfigCredentials can accept
// either shape in the same config.json field without a schema change (the
// same convention wpa_passphrase-style tooling and NetworkManager use).
func isHexPSK(s string) bool {
	if len(s) != 64 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}
