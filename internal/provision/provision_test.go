package provision

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixtureDir points at a scenario captured off a real Raspberry Pi Imager
// v2.0.10 run (see testdata/imager-2.0.10/capture-notes.md). Every value
// asserted below was read directly out of the committed fixture files, not
// invented — per the bean's "parse ONLY against the committed fixtures"
// rule.
func fixtureDir(scenario string) string {
	return filepath.Join("testdata", "imager-2.0.10", scenario)
}

func TestReadWifiHostnameFixture(t *testing.T) {
	var logs []string
	result := Read(fixtureDir("wifi-hostname"), collectLog(&logs))

	if result.Hostname != "fixture-one" {
		t.Errorf("Hostname = %q, want %q", result.Hostname, "fixture-one")
	}
	if len(result.Wifi) != 1 {
		t.Fatalf("Wifi = %+v, want exactly one network", result.Wifi)
	}
	want := WifiNetwork{
		SSID:     "demo",
		Password: "3103b87804bb44f3ae397ee94737e1db750128c9300c083b1c518647484f6cc5",
	}
	if result.Wifi[0] != want {
		t.Errorf("Wifi[0] = %+v, want %+v", result.Wifi[0], want)
	}
	if result.FirstrunPresent {
		t.Error("FirstrunPresent = true, this fixture has no firstrun.sh")
	}
}

func TestReadHostnameOnlyFixture(t *testing.T) {
	// No network-config file exists in this scenario at all (WiFi was left
	// unconfigured in the dialog) — Read must not error or invent a network.
	var logs []string
	result := Read(fixtureDir("hostname-only"), collectLog(&logs))

	if result.Hostname != "fixture-one" {
		t.Errorf("Hostname = %q, want %q", result.Hostname, "fixture-one")
	}
	if len(result.Wifi) != 0 {
		t.Errorf("Wifi = %+v, want none (network-config was never written)", result.Wifi)
	}

	// user-data in this fixture also carries a runcmd block (rfkill
	// unblock) that gosd-init doesn't consume; it must be summarized, not
	// silently dropped without a trace.
	if !containsSubstring(logs, "runcmd") {
		t.Errorf("logs = %v, want a summary line mentioning the ignored runcmd field", logs)
	}
}

func TestReadEverythingFixture(t *testing.T) {
	// This scenario configures a hidden network, a user account, SSH, and
	// locale — gosd-init only consumes hostname and the WiFi network;
	// everything else must be ignored without error.
	var logs []string
	result := Read(fixtureDir("everything"), collectLog(&logs))

	if result.Hostname != "fixture-one" {
		t.Errorf("Hostname = %q, want %q", result.Hostname, "fixture-one")
	}
	if len(result.Wifi) != 1 {
		t.Fatalf("Wifi = %+v, want exactly one network", result.Wifi)
	}
	want := WifiNetwork{
		SSID:     "hidden-network",
		Password: "b1b7cc4dd077d74bc3a830ebad9cd885add86ada76f183e6343667a3f70f93b9",
		Hidden:   true,
	}
	if result.Wifi[0] != want {
		t.Errorf("Wifi[0] = %+v, want %+v", result.Wifi[0], want)
	}
	if !containsSubstring(logs, "user") {
		t.Errorf("logs = %v, want a summary line mentioning the ignored user field", logs)
	}
}

func TestReadIgnoresMissingFilesSilently(t *testing.T) {
	// An empty boot partition (no provisioning at all) is the overwhelming
	// common case and must never be logged as a problem.
	var logs []string
	result := Read(t.TempDir(), collectLog(&logs))

	if result.Hostname != "" || result.Wifi != nil || result.FirstrunPresent {
		t.Errorf("Read() = %+v, want the zero Result", result)
	}
	if len(logs) != 0 {
		t.Errorf("logs = %v, want none for an empty boot partition", logs)
	}
}

func TestReadDetectsFirstrunShWithoutParsingIt(t *testing.T) {
	dir := t.TempDir()
	// A shell script gosd-init must never execute or parse: if it did,
	// this test would need to assert on extracted fields instead of just
	// presence.
	writeFile(t, dir, "firstrun.sh", "#!/bin/sh\nimager_custom set_hostname evil\n")

	var logs []string
	result := Read(dir, collectLog(&logs))

	if !result.FirstrunPresent {
		t.Error("FirstrunPresent = false, want true")
	}
	if result.Hostname != "" {
		t.Errorf("Hostname = %q, want empty — firstrun.sh must never be parsed", result.Hostname)
	}
	if !containsSubstring(logs, "gosd.toml") {
		t.Errorf("logs = %v, want a line pointing the user at gosd.toml", logs)
	}
}

func TestReadFallsBackOnMalformedUserData(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "user-data", "hostname: [this is not a scalar\n")

	var logs []string
	result := Read(dir, collectLog(&logs))

	if result.Hostname != "" {
		t.Errorf("Hostname = %q, want empty for malformed YAML", result.Hostname)
	}
	if !containsSubstring(logs, "user-data") {
		t.Errorf("logs = %v, want a warning naming user-data", logs)
	}
}

func TestReadFallsBackOnMalformedNetworkConfig(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "network-config", "network: [nope\n")

	var logs []string
	result := Read(dir, collectLog(&logs))

	if result.Wifi != nil {
		t.Errorf("Wifi = %+v, want none for malformed YAML", result.Wifi)
	}
	if !containsSubstring(logs, "network-config") {
		t.Errorf("logs = %v, want a warning naming network-config", logs)
	}
}

func TestParseUserDataPlainDocumentHasNoHostname(t *testing.T) {
	hostname, ignored, err := parseUserData(nil)
	if err != nil {
		t.Fatalf("parseUserData(nil) error = %v", err)
	}
	if hostname != "" || ignored != nil {
		t.Errorf("parseUserData(nil) = %q, %v, want empty", hostname, ignored)
	}
}

func TestParseUserDataRejectsNonMapping(t *testing.T) {
	_, _, err := parseUserData([]byte("- just\n- a\n- list\n"))
	if err == nil {
		t.Fatal("parseUserData() error = nil, want an error for a non-mapping document")
	}
}

func TestParseNetworkConfigPlaintextPassphraseIsPassedThroughUnexamined(t *testing.T) {
	// Hand-written network-config files (unlike anything Imager itself
	// writes, which always pre-hashes — see docs/provisioning-formats.md
	// §2) may carry a plaintext passphrase directly. This package must not
	// try to distinguish the two shapes itself (that's wifiup's job); it
	// just needs to carry the string through faithfully.
	data := []byte(`
network:
  version: 2
  wifis:
    wlan0:
      access-points:
        "home-network":
          password: "correct horse battery staple"
`)
	networks, err := parseNetworkConfig(data)
	if err != nil {
		t.Fatalf("parseNetworkConfig() error = %v", err)
	}
	want := []WifiNetwork{{SSID: "home-network", Password: "correct horse battery staple"}}
	if len(networks) != 1 || networks[0] != want[0] {
		t.Errorf("parseNetworkConfig() = %+v, want %+v", networks, want)
	}
}

func TestParseNetworkConfigOpenNetworkHasNoPassword(t *testing.T) {
	data := []byte(`
network:
  version: 2
  wifis:
    wlan0:
      access-points:
        "open-guest-network": {}
`)
	networks, err := parseNetworkConfig(data)
	if err != nil {
		t.Fatalf("parseNetworkConfig() error = %v", err)
	}
	want := []WifiNetwork{{SSID: "open-guest-network"}}
	if len(networks) != 1 || networks[0] != want[0] {
		t.Errorf("parseNetworkConfig() = %+v, want %+v (empty password means open)", networks, want)
	}
}

func TestParseNetworkConfigMultipleAccessPointsPreserveFileOrder(t *testing.T) {
	// yaml.v3 unmarshaling straight into a Go map would scramble this
	// order (map iteration is randomized); parseNetworkConfig must walk
	// the raw node tree to keep it, per the "take all, ordered" rule.
	data := []byte(`
network:
  version: 2
  wifis:
    wlan0:
      access-points:
        "third-ssid":
          password: "third-pass"
        "first-ssid":
          password: "first-pass"
        "second-ssid":
          password: "second-pass"
`)
	networks, err := parseNetworkConfig(data)
	if err != nil {
		t.Fatalf("parseNetworkConfig() error = %v", err)
	}
	wantOrder := []string{"third-ssid", "first-ssid", "second-ssid"}
	if len(networks) != len(wantOrder) {
		t.Fatalf("parseNetworkConfig() returned %d networks, want %d", len(networks), len(wantOrder))
	}
	for i, ssid := range wantOrder {
		if networks[i].SSID != ssid {
			t.Errorf("networks[%d].SSID = %q, want %q (order not preserved)", i, networks[i].SSID, ssid)
		}
	}
}

func TestParseNetworkConfigNoWifiSectionIsNotAnError(t *testing.T) {
	data := []byte(`
network:
  version: 2
  ethernets:
    eth0:
      dhcp4: true
`)
	networks, err := parseNetworkConfig(data)
	if err != nil {
		t.Fatalf("parseNetworkConfig() error = %v", err)
	}
	if networks != nil {
		t.Errorf("parseNetworkConfig() = %+v, want nil for an Ethernet-only config", networks)
	}
}

func TestParseNetworkConfigRejectsMalformedYAML(t *testing.T) {
	_, err := parseNetworkConfig([]byte("network: [this is not closed\n"))
	if err == nil {
		t.Fatal("parseNetworkConfig() error = nil, want an error for malformed YAML")
	}
}

// collectLog returns a log function that appends every formatted line to
// *logs, so tests can assert on gosd-init's log output without depending on
// its exact wording beyond a targeted substring.
func collectLog(logs *[]string) func(format string, args ...any) {
	return func(format string, args ...any) {
		*logs = append(*logs, fmt.Sprintf(format, args...))
	}
}

func containsSubstring(logs []string, substr string) bool {
	for _, l := range logs {
		if strings.Contains(l, substr) {
			return true
		}
	}
	return false
}

func writeFile(t *testing.T, dir, name, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o644); err != nil {
		t.Fatalf("writing fixture %s: %v", name, err)
	}
}
