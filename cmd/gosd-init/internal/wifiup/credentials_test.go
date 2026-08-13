package wifiup

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/jphastings/gosd/internal/initcfg"
)

func TestConfigCredentialsNoSSIDMeansNotConfigured(t *testing.T) {
	src := ConfigCredentials{Wifi: initcfg.Wifi{}}
	_, ok, err := src.Credentials()
	if err != nil {
		t.Fatalf("Credentials() error = %v", err)
	}
	if ok {
		t.Error("Credentials() ok = true with no SSID, want false")
	}
}

func TestConfigCredentialsEmptyPassphraseMeansOpen(t *testing.T) {
	src := ConfigCredentials{Wifi: initcfg.Wifi{SSID: "guest-net"}}
	creds, ok, err := src.Credentials()
	if err != nil {
		t.Fatalf("Credentials() error = %v", err)
	}
	if !ok || !creds.Open || creds.SSID != "guest-net" {
		t.Errorf("Credentials() = %+v, ok=%v, want open network %q", creds, ok, "guest-net")
	}
}

func TestConfigCredentialsPlaintextPassphraseDerivesPSK(t *testing.T) {
	src := ConfigCredentials{Wifi: initcfg.Wifi{SSID: "IEEE", Passphrase: "password"}}
	creds, ok, err := src.Credentials()
	if err != nil {
		t.Fatalf("Credentials() error = %v", err)
	}
	if !ok || creds.Open {
		t.Fatalf("Credentials() = %+v, ok=%v, want a PSK network", creds, ok)
	}
	want, _ := DerivePSK("password", "IEEE")
	if creds.PSK != want {
		t.Errorf("Credentials().PSK = %x, want %x (derived directly)", creds.PSK, want)
	}
}

func TestConfigCredentialsPreHashedHexPSKIsUsedDirectly(t *testing.T) {
	derived, _ := DerivePSK("some-passphrase-nobody-should-see-again", "office")
	pskHex := hex.EncodeToString(derived[:])

	src := ConfigCredentials{Wifi: initcfg.Wifi{SSID: "office", Passphrase: pskHex}}
	creds, ok, err := src.Credentials()
	if err != nil {
		t.Fatalf("Credentials() error = %v", err)
	}
	if !ok || creds.Open {
		t.Fatalf("Credentials() = %+v, ok=%v, want a PSK network", creds, ok)
	}
	if creds.PSK != derived {
		t.Errorf("Credentials().PSK = %x, want %x (pre-hashed value used as-is, not re-derived)", creds.PSK, derived)
	}

	// A passphrase that merely happens to be a plaintext string is never
	// mistaken for hex, and vice versa: prove the two 64-char forms produce
	// different keys, so we know the branch was actually taken by shape.
	asPassphrase, _ := DerivePSK(pskHex, "office")
	if asPassphrase == derived {
		t.Fatal("test fixture is degenerate: treating the hex string as a passphrase coincidentally produced the same key")
	}
}

func TestConfigCredentialsCardTakesPrecedenceOverConfigJSON(t *testing.T) {
	src := ConfigCredentials{
		Wifi: initcfg.Wifi{SSID: "baked-in-network", Passphrase: "baked-in-password"},
		Card: Wifi{SSID: "card-network", Passphrase: "card-password"},
	}
	creds, ok, err := src.Credentials()
	if err != nil {
		t.Fatalf("Credentials() error = %v", err)
	}
	want, _ := DerivePSK("card-password", "card-network")
	if !ok || creds.SSID != "card-network" || creds.PSK != want {
		t.Errorf("Credentials() = %+v, ok=%v, want the network named on the card to win", creds, ok)
	}
}

func TestConfigCredentialsFallsBackToConfigJSONWhenTheCardNamesNoNetwork(t *testing.T) {
	src := ConfigCredentials{
		Wifi: initcfg.Wifi{SSID: "baked-in-network", Passphrase: "baked-in-password"},
	}
	creds, ok, err := src.Credentials()
	if err != nil {
		t.Fatalf("Credentials() error = %v", err)
	}
	want, _ := DerivePSK("baked-in-password", "baked-in-network")
	if !ok || creds.SSID != "baked-in-network" || creds.PSK != want {
		t.Errorf("Credentials() = %+v, ok=%v, want the config.json network as fallback", creds, ok)
	}
}

func TestConfigCredentialsCardOpenNetworkHasNoPassword(t *testing.T) {
	src := ConfigCredentials{Card: Wifi{SSID: "open-guest-network"}}
	creds, ok, err := src.Credentials()
	if err != nil {
		t.Fatalf("Credentials() error = %v", err)
	}
	if !ok || !creds.Open || creds.SSID != "open-guest-network" {
		t.Errorf("Credentials() = %+v, ok=%v, want an open network %q", creds, ok, "open-guest-network")
	}
}

func TestConfigCredentialsCardPreHashedHexPSKIsUsedDirectly(t *testing.T) {
	// This is the shape Raspberry Pi Imager always writes (see
	// docs/provisioning-formats.md §2), and its wizard's answers reach the
	// card as ordinary settings — so a pre-hashed key must be accepted
	// as-is here, not re-derived, wherever the card got it from.
	derived, _ := DerivePSK("some-passphrase-nobody-should-see-again", "office")
	pskHex := hex.EncodeToString(derived[:])

	src := ConfigCredentials{Card: Wifi{SSID: "office", Passphrase: pskHex}}
	creds, ok, err := src.Credentials()
	if err != nil {
		t.Fatalf("Credentials() error = %v", err)
	}
	if !ok || creds.Open || creds.PSK != derived {
		t.Errorf("Credentials() = %+v, ok=%v, want the pre-hashed PSK used as-is", creds, ok)
	}
}

func TestConfigCredentialsTreats64CharNonHexAsPassphrase(t *testing.T) {
	// Right length to be mistaken for a pre-hashed PSK, but not valid hex:
	// isHexPSK's shape check must reject it, so it's derived as an
	// (unusual but valid) plaintext passphrase instead of erroring out.
	unusual := strings.Repeat("z", 64)
	src := ConfigCredentials{Wifi: initcfg.Wifi{SSID: "office", Passphrase: unusual}}
	creds, ok, err := src.Credentials()
	if err != nil {
		t.Fatalf("Credentials() error = %v, want the passphrase branch to accept it", err)
	}
	want, _ := DerivePSK(unusual, "office")
	if !ok || creds.PSK != want {
		t.Errorf("Credentials().PSK = %x, want %x (derived as a passphrase)", creds.PSK, want)
	}
}
