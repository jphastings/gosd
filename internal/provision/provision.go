// Package provision reads what Raspberry Pi Imager (or a hand-editing user)
// left on the GOSD-BOOT partition and extracts the subset gosd-init
// consumes: a hostname (from cloud-init's user-data) and WiFi access points
// (from cloud-init's network-config).
//
// Per the locked end-user-flashing decision (see root CLAUDE.md and
// docs/provisioning-formats.md), the flagship flashing path is Imager's
// custom-repository catalog flow with init_format: "cloudinit", so this
// package only ever needs to understand cloud-init's YAML — never
// firstrun.sh (the systemd/legacy mechanism), which gosd-init deliberately
// never parses or executes. gosd.toml (see internal/gosdtoml) is parsed
// separately and always wins over anything found here; this package only
// produces the cloud-init tier of that precedence chain
// (gosd.toml > cloud-init > baked config.json).
//
// Every parse here is best-effort: a missing, unreadable, or malformed file
// is logged and skipped, never returned as an error, because a bad
// provisioning file on the boot partition must never stop gosd-init from
// booting the app.
package provision

import (
	"os"
	"path/filepath"
	"strings"
)

// Result is what gosd-init consumes from cloud-init provisioning found on
// the GOSD-BOOT partition.
type Result struct {
	// Hostname is the hostname cloud-init's user-data requested, or "" if
	// user-data was absent, unreadable, malformed, or didn't set one.
	Hostname string

	// Wifi lists every access point cloud-init's network-config named,
	// in file order. gosd-init only ever joins one network at a time (see
	// wifiup), so only the first entry is used — the rest are kept here
	// only so callers can log what else was found.
	Wifi []WifiNetwork

	// FirstrunPresent is true when a firstrun.sh file was found on the
	// boot partition. Per the locked flashing-path decision, gosd-init
	// never parses or executes it; this only lets the caller log one line
	// pointing the user at gosd.toml instead.
	FirstrunPresent bool
}

// WifiNetwork is a single access point named under network-config's
// wifis.*.access-points map.
type WifiNetwork struct {
	SSID string

	// Password is either a plaintext passphrase or a pre-hashed 64-hex
	// PBKDF2 PSK (the form Raspberry Pi Imager always writes), passed
	// through exactly as found — this package never inspects its shape.
	// wifiup.ConfigCredentials already distinguishes the two by shape
	// (see wifiup.DerivePSK / wifiup.ParsePSKHex), so accepting this
	// value into that same chain reuses that logic rather than
	// duplicating it. Empty means an open network.
	Password string

	// Hidden mirrors network-config's "hidden: true" flag. gosd-init's
	// current WiFi association (see wifiup) does not yet special-case
	// hidden networks (no directed probe request), so this is carried
	// through for completeness and future use, not yet acted on.
	Hidden bool
}

const (
	userDataFile      = "user-data"
	networkConfigFile = "network-config"
	firstrunFile      = "firstrun.sh"
)

// Read looks for cloud-init's user-data and network-config, and for a
// firstrun.sh, directly inside bootDir (the mounted GOSD-BOOT partition),
// and extracts what gosd-init consumes. Missing files are silent — most
// images will never carry any of them — but a present, unreadable, or
// malformed file is logged through log and then skipped, falling back to
// whatever the next-lower-precedence source provides.
//
// meta-data is deliberately never read: every captured Imager v2.0.10
// fixture shows it containing only cloud-init's own instance-id
// bookkeeping field (required so the NoCloud datasource treats a
// regenerated seed as fresh), nothing gosd-init consumes — see
// docs/provisioning-formats.md.
func Read(bootDir string, log func(format string, args ...any)) Result {
	var result Result

	if data, ok := readOptional(filepath.Join(bootDir, userDataFile), userDataFile, log); ok {
		hostname, ignored, err := parseUserData(data)
		if err != nil {
			log("parsing cloud-init %s failed, ignoring it: %v", userDataFile, err)
		} else {
			result.Hostname = hostname
			if len(ignored) > 0 {
				log("cloud-init %s: gosd-init only consumes hostname; ignoring %d other field(s) (%s)", userDataFile, len(ignored), strings.Join(ignored, ", "))
			}
		}
	}

	if data, ok := readOptional(filepath.Join(bootDir, networkConfigFile), networkConfigFile, log); ok {
		networks, err := parseNetworkConfig(data)
		if err != nil {
			log("parsing cloud-init %s failed, ignoring it: %v", networkConfigFile, err)
		} else {
			result.Wifi = networks
		}
	}

	if _, err := os.Stat(filepath.Join(bootDir, firstrunFile)); err == nil {
		result.FirstrunPresent = true
		log("%s found on the boot partition; gosd-init never parses or executes it — use gosd.toml to configure this device instead", firstrunFile)
	}

	return result
}

// readOptional reads path, treating a missing file as a silent, expected
// case (ok=false, no log) and any other read error as worth surfacing
// (still ok=false, but logged) since it means a file is present but
// somehow inaccessible.
func readOptional(path, name string, log func(format string, args ...any)) ([]byte, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			log("reading %s failed, ignoring it: %v", name, err)
		}
		return nil, false
	}
	return data, true
}
