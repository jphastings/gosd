package wifiup

import (
	"github.com/jphastings/gosd/internal/gosdtoml"
	"github.com/jphastings/gosd/internal/initcfg"
	"github.com/jphastings/gosd/internal/provision"
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
// CredentialSource — gosd-init's v0.1 credential source — later extended
// (gosd-pctc) with the two higher-precedence sources the locked precedence
// chain adds on top of it (see docs/provisioning-formats.md):
//
//	gosd.toml  >  cloud-init (Imager provisioning)  >  config.json
//
// GosdToml, if set (non-empty SSID), always wins. Otherwise Provision, if
// it named at least one network, wins over the baked-in Wifi. Both are
// zero by default, so callers that only have config.json — including
// existing tests — don't need to change.
type ConfigCredentials struct {
	Wifi     initcfg.Wifi
	GosdToml gosdtoml.Wifi

	// Provision holds every WiFi network cloud-init's network-config
	// named, in file order (see internal/provision). Only the first is
	// ever joined — gosd-init supports one active WiFi network at a
	// time — the rest exist only so a caller can log what else was
	// found; a nil/empty slice means no cloud-init network-config was
	// found (or it named no WiFi at all).
	Provision []provision.WifiNetwork
}

// Credentials resolves the effective ssid/passphrase pair — from GosdToml
// if it names a network, else the first entry of Provision if there is
// one, else Wifi (config.json) — into a Credentials value.
//
// The passphrase does double duty, distinguished by shape rather than a
// separate schema field (initcfg.Wifi's, gosdtoml.Wifi's, and
// provision.WifiNetwork's schemas are all locked): a 64-hex-character
// value is treated as a pre-hashed PSK — the form Raspberry Pi Imager's
// cloud-init provisioning always writes, so a plaintext password never has
// to be baked onto the image — and anything else is treated as a
// plaintext passphrase, run through DerivePSK. An empty passphrase with a
// non-empty SSID means an open network.
//
// None of the three schemas has a field to express WPA3/EAP (nor any
// other security mode) at all, so there is currently no input that
// reaches the Unsupported path below; it exists so that if any of them
// grows a security mode field later, there's an obvious place to reject
// it clearly instead of misinterpreting it as PSK or open.
func (c ConfigCredentials) Credentials() (Credentials, bool, error) {
	wifi := c.Wifi
	if len(c.Provision) > 0 {
		wifi = initcfg.Wifi{SSID: c.Provision[0].SSID, Passphrase: c.Provision[0].Password}
	}
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
