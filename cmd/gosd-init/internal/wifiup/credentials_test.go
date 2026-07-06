package wifiup

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/jphastings/gosd/internal/gosdtoml"
	"github.com/jphastings/gosd/internal/initcfg"
	"github.com/jphastings/gosd/internal/provision"
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

func TestConfigCredentialsGosdTomlTakesPrecedenceOverConfigJSON(t *testing.T) {
	src := ConfigCredentials{
		Wifi:     initcfg.Wifi{SSID: "baked-in-network", Passphrase: "baked-in-password"},
		GosdToml: gosdtoml.Wifi{SSID: "hand-edited-network", Passphrase: "hand-edited-password"},
	}
	creds, ok, err := src.Credentials()
	if err != nil {
		t.Fatalf("Credentials() error = %v", err)
	}
	want, _ := DerivePSK("hand-edited-password", "hand-edited-network")
	if !ok || creds.SSID != "hand-edited-network" || creds.PSK != want {
		t.Errorf("Credentials() = %+v, ok=%v, want the gosd.toml network to win", creds, ok)
	}
}

func TestConfigCredentialsFallsBackToConfigJSONWhenGosdTomlHasNoSSID(t *testing.T) {
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

func TestConfigCredentialsProvisionTakesPrecedenceOverConfigJSON(t *testing.T) {
	src := ConfigCredentials{
		Wifi:      initcfg.Wifi{SSID: "baked-in-network", Passphrase: "baked-in-password"},
		Provision: []provision.WifiNetwork{{SSID: "cloud-init-network", Password: "cloud-init-password"}},
	}
	creds, ok, err := src.Credentials()
	if err != nil {
		t.Fatalf("Credentials() error = %v", err)
	}
	want, _ := DerivePSK("cloud-init-password", "cloud-init-network")
	if !ok || creds.SSID != "cloud-init-network" || creds.PSK != want {
		t.Errorf("Credentials() = %+v, ok=%v, want the cloud-init network to win over config.json", creds, ok)
	}
}

func TestConfigCredentialsGosdTomlTakesPrecedenceOverProvision(t *testing.T) {
	src := ConfigCredentials{
		Wifi:      initcfg.Wifi{SSID: "baked-in-network", Passphrase: "baked-in-password"},
		Provision: []provision.WifiNetwork{{SSID: "cloud-init-network", Password: "cloud-init-password"}},
		GosdToml:  gosdtoml.Wifi{SSID: "hand-edited-network", Passphrase: "hand-edited-password"},
	}
	creds, ok, err := src.Credentials()
	if err != nil {
		t.Fatalf("Credentials() error = %v", err)
	}
	want, _ := DerivePSK("hand-edited-password", "hand-edited-network")
	if !ok || creds.SSID != "hand-edited-network" || creds.PSK != want {
		t.Errorf("Credentials() = %+v, ok=%v, want the gosd.toml network to win over cloud-init", creds, ok)
	}
}

func TestConfigCredentialsProvisionOnlyUsesFirstNetwork(t *testing.T) {
	// gosd-init only ever joins one WiFi network; a network-config naming
	// several access points must not be treated as an error, but only the
	// first is ever used.
	src := ConfigCredentials{
		Provision: []provision.WifiNetwork{
			{SSID: "first-network", Password: "first-password"},
			{SSID: "second-network", Password: "second-password"},
		},
	}
	creds, ok, err := src.Credentials()
	if err != nil {
		t.Fatalf("Credentials() error = %v", err)
	}
	if !ok || creds.SSID != "first-network" {
		t.Errorf("Credentials().SSID = %q, ok=%v, want %q (the first entry)", creds.SSID, ok, "first-network")
	}
}

func TestConfigCredentialsProvisionOpenNetworkHasNoPassword(t *testing.T) {
	src := ConfigCredentials{
		Provision: []provision.WifiNetwork{{SSID: "open-guest-network"}},
	}
	creds, ok, err := src.Credentials()
	if err != nil {
		t.Fatalf("Credentials() error = %v", err)
	}
	if !ok || !creds.Open || creds.SSID != "open-guest-network" {
		t.Errorf("Credentials() = %+v, ok=%v, want an open network %q", creds, ok, "open-guest-network")
	}
}

func TestConfigCredentialsProvisionPreHashedHexPSKIsUsedDirectly(t *testing.T) {
	// This is the shape Raspberry Pi Imager always writes (see
	// docs/provisioning-formats.md §2) — proving it's accepted directly,
	// not re-derived, is what lets internal/provision pass the value
	// through unexamined rather than duplicating this detection.
	derived, _ := DerivePSK("some-passphrase-nobody-should-see-again", "office")
	pskHex := hex.EncodeToString(derived[:])

	src := ConfigCredentials{
		Provision: []provision.WifiNetwork{{SSID: "office", Password: pskHex}},
	}
	creds, ok, err := src.Credentials()
	if err != nil {
		t.Fatalf("Credentials() error = %v", err)
	}
	if !ok || creds.Open || creds.PSK != derived {
		t.Errorf("Credentials() = %+v, ok=%v, want the pre-hashed PSK used as-is", creds, ok)
	}
}

func TestConfigCredentialsProvisionHiddenNetworkSetsHidden(t *testing.T) {
	src := ConfigCredentials{
		Provision: []provision.WifiNetwork{{SSID: "shy-network", Password: "shy-password", Hidden: true}},
	}
	creds, ok, err := src.Credentials()
	if err != nil {
		t.Fatalf("Credentials() error = %v", err)
	}
	if !ok || !creds.Hidden || creds.SSID != "shy-network" {
		t.Errorf("Credentials() = %+v, ok=%v, want Hidden=true for %q", creds, ok, "shy-network")
	}
}

func TestConfigCredentialsProvisionNonHiddenNetworkLeavesHiddenFalse(t *testing.T) {
	src := ConfigCredentials{
		Provision: []provision.WifiNetwork{{SSID: "visible-network", Password: "visible-password"}},
	}
	creds, ok, err := src.Credentials()
	if err != nil {
		t.Fatalf("Credentials() error = %v", err)
	}
	if !ok || creds.Hidden {
		t.Errorf("Credentials() = %+v, ok=%v, want Hidden=false", creds, ok)
	}
}

func TestConfigCredentialsGosdTomlOverridingHiddenProvisionClearsHidden(t *testing.T) {
	// gosd.toml's schema has no hidden field (locked), and it takes
	// precedence over cloud-init provisioning — so once it wins, the
	// hidden-ness of the network it replaced must not leak through.
	src := ConfigCredentials{
		Provision: []provision.WifiNetwork{{SSID: "shy-network", Password: "shy-password", Hidden: true}},
		GosdToml:  gosdtoml.Wifi{SSID: "hand-edited-network", Passphrase: "hand-edited-password"},
	}
	creds, ok, err := src.Credentials()
	if err != nil {
		t.Fatalf("Credentials() error = %v", err)
	}
	if !ok || creds.SSID != "hand-edited-network" || creds.Hidden {
		t.Errorf("Credentials() = %+v, ok=%v, want the gosd.toml network with Hidden=false", creds, ok)
	}
}

func TestConfigCredentialsConfigJSONNeverHidden(t *testing.T) {
	// initcfg.Wifi's schema has no hidden field (locked): config.json can
	// never mark a network hidden, regardless of what else is set.
	src := ConfigCredentials{Wifi: initcfg.Wifi{SSID: "baked-in-network", Passphrase: "baked-in-password"}}
	creds, ok, err := src.Credentials()
	if err != nil {
		t.Fatalf("Credentials() error = %v", err)
	}
	if !ok || creds.Hidden {
		t.Errorf("Credentials() = %+v, ok=%v, want Hidden=false", creds, ok)
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
