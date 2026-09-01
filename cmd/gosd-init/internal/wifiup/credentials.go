package wifiup

import (
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

// Wifi is the wireless network named on the card: the wifi/ssid and
// wifi/passphrase settings of its config tree, read as they are. Both
// fields empty means the card names no network, which is the ordinary case
// for a device that reaches its network over Ethernet or was built with
// credentials baked in.
type Wifi struct {
	SSID       string
	Passphrase string
}

// ConfigCredentials is gosd-init's CredentialSource: the network named on
// the card, and the one baked into config.json as the fallback for a card
// that names none. There is no third tier — an Imager wizard's answers
// reach this as ordinary card settings, written into the config tree when
// their seed was consumed (see boot's consumeCloudInit) — and no priority
// list to remember: Card, if it names a network, is the network.
type ConfigCredentials struct {
	Wifi initcfg.Wifi
	Card Wifi
}

// Credentials resolves the effective ssid/passphrase pair — the card's if
// it names a network, else config.json's — into a Credentials value.
//
// The passphrase does double duty, distinguished by shape rather than a
// separate field: a 64-hex-character value is treated as a pre-hashed PSK —
// the form Raspberry Pi Imager's cloud-init provisioning always writes, so
// a plaintext password never has to be baked onto the image — and anything
// else is treated as a plaintext passphrase, run through DerivePSK. An
// empty passphrase with a non-empty SSID means an open network.
//
// Neither source has a field to express WPA3/EAP (nor any other security
// mode) at all, so there is currently no input that reaches the Unsupported
// path below; it exists so that if either grows a security mode field
// later, there's an obvious place to reject it clearly instead of
// misinterpreting it as PSK or open.
func (c ConfigCredentials) Credentials() (Credentials, bool, error) {
	wifi := c.Wifi
	if c.Card.SSID != "" {
		wifi = initcfg.Wifi{SSID: c.Card.SSID, Passphrase: c.Card.Passphrase}
	}

	if wifi.SSID == "" {
		return Credentials{}, false, nil
	}
	creds, err := resolveCredentials(wifi.SSID, wifi.Passphrase)
	if err != nil {
		return Credentials{}, false, err
	}
	return creds, true, nil
}

// resolveCredentials turns an ssid/passphrase pair into a Credentials,
// resolving the passphrase exactly as ConfigCredentials.Credentials always
// has: a 64-hex-character value is a pre-hashed PSK, anything else is run
// through DerivePSK, and an empty passphrase means an open network. Shared
// with the runtime-join reconciler (see handleRequest) so a request's
// ssid/passphrase pair is resolved on the same terms as a boot-time one,
// not a second copy of this logic.
func resolveCredentials(ssid, passphrase string) (Credentials, error) {
	if passphrase == "" {
		return Credentials{SSID: ssid, Open: true}, nil
	}

	var (
		psk [32]byte
		err error
	)
	if isHexPSK(passphrase) {
		psk, err = ParsePSKHex(passphrase)
	} else {
		psk, err = DerivePSK(passphrase, ssid)
	}
	if err != nil {
		return Credentials{}, err
	}
	return Credentials{SSID: ssid, PSK: psk}, nil
}
