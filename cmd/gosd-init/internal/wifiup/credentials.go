package wifiup

import (
	"github.com/jphastings/gosd/internal/gosdtoml"
	"github.com/jphastings/gosd/internal/initcfg"
)

// Credentials describes the single network wifiup should join.
type Credentials struct {
	SSID string
	// Open is true for a network with no security at all. Mutually
	// exclusive with PSK being meaningful.
	Open bool
	// PSK is the already-resolved 256-bit WPA2 pairwise master key —
	// either derived from a passphrase via DerivePSK, or decoded
	// directly from a pre-hashed hex value via ParsePSKHex. Zero when
	// Open is true.
	PSK [32]byte

	// Unsupported, if non-empty, names a security mode config.json (or a
	// future CredentialSource) requested that gosd-init cannot join —
	// WPA3 and 802.1X/EAP are out of scope through v0.x (locked
	// decision). wifiup.Run logs this clearly and skips WiFi bring-up
	// entirely rather than retrying forever against a network it can
	// never join.
	Unsupported string
}

// ConfigCredentials adapts config.json's initcfg.Wifi block into a
// CredentialSource — gosd-init's v0.1 credential source. Behind the
// CredentialSource interface so v0.2's Imager provisioning can supply
// credentials from a different origin without any change to wifiup.
//
// GosdToml, if set (non-empty SSID), takes precedence over Wifi entirely:
// gosd.toml is the hand-editable file on the boot partition, so a network a
// user has typed in there is expected to win over whatever was baked into
// config.json at build time. It's zero by default, so callers that only
// have config.json — including existing tests — don't need to change.
type ConfigCredentials struct {
	Wifi     initcfg.Wifi
	GosdToml gosdtoml.Wifi
}

// Credentials resolves the effective wifi.ssid/wifi.passphrase pair — from
// GosdToml if it names a network, otherwise from Wifi (config.json) — into
// a Credentials value.
//
// The passphrase does double duty, distinguished by shape rather than a
// separate schema field (both initcfg.Wifi's and gosdtoml.Wifi's schemas
// are locked): a 64-hex-character value is treated as a pre-hashed PSK —
// the form v0.2's Imager provisioning is expected to write, so a plaintext
// password never has to be baked onto the image — and anything else is
// treated as a plaintext passphrase, run through DerivePSK. An empty
// passphrase with a non-empty SSID means an open network.
//
// Neither schema has a field to express WPA3/EAP (nor any other security
// mode) at all, so there is currently no input that reaches the
// Unsupported path below; it exists so that if either schema grows a
// security mode field later, there's an obvious place to reject it clearly
// instead of misinterpreting it as PSK or open.
func (c ConfigCredentials) Credentials() (Credentials, bool, error) {
	wifi := c.Wifi
	if c.GosdToml.SSID != "" {
		wifi = initcfg.Wifi{SSID: c.GosdToml.SSID, Passphrase: c.GosdToml.Passphrase}
	}

	if wifi.SSID == "" {
		return Credentials{}, false, nil
	}
	if wifi.Passphrase == "" {
		return Credentials{SSID: wifi.SSID, Open: true}, true, nil
	}

	var (
		psk [32]byte
		err error
	)
	if isHexPSK(wifi.Passphrase) {
		psk, err = ParsePSKHex(wifi.Passphrase)
	} else {
		psk, err = DerivePSK(wifi.Passphrase, wifi.SSID)
	}
	if err != nil {
		return Credentials{}, false, err
	}
	return Credentials{SSID: wifi.SSID, PSK: psk}, true, nil
}
