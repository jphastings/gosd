// Package provision reads what Raspberry Pi Imager (or a hand-editing user)
// left on the boot partition and extracts the subset gosd-init
// consumes: a hostname (from cloud-init's user-data) and WiFi access points
// (from cloud-init's network-config).
//
// Per the locked end-user-flashing decision (see root CLAUDE.md and
// docs/provisioning-formats.md), the flagship flashing path is Imager's
// custom-repository catalog flow with init_format: "cloudinit", so this
// package only ever needs to understand cloud-init's YAML — never
// firstrun.sh (the systemd/legacy mechanism), which gosd-init deliberately
// never parses or executes.
//
// There is no precedence chain to place this in: a seed is CONSUMED, not
// consulted. gosd-init deletes it and writes what it asked for into the
// card's config/ tree (see internal/configtree and
// cmd/gosd-init/internal/cardconfig), so a wizard's answers become ordinary
// settings — visible in the same files a person edits by hand, and carried
// across a reflash by the same mechanism.
//
// Every parse here is best-effort: a missing, unreadable, or malformed file
// is logged and skipped, never returned as an error, because a bad
// provisioning file on the boot partition must never stop gosd-init from
// booting the app.
package provision

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jphastings/gosd/internal/configtree"
)

// Result is what gosd-init consumes from cloud-init provisioning found on
// the boot partition.
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
	// pointing the user at the config tree instead.
	FirstrunPresent bool

	// SeedFiles names the cloud-init seed files actually found on the boot
	// partition, in the fixed order below. The caller consumes a seed by
	// DELETING it — and making that deletion durable — before writing what
	// it asked for into the card's config tree, so a wizard's answers
	// become ordinary settings that survive a reflash instead of a file
	// that re-applies itself over every later hand-edit (locked, epic
	// gosd-rw6n). Empty for the overwhelmingly common case of a card with
	// no seed on it at all: every boot after the first.
	SeedFiles []string
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
	metaDataFile      = "meta-data"
	firstrunFile      = "firstrun.sh"
)

// Read looks for cloud-init's user-data and network-config, and for a
// firstrun.sh, directly inside bootDir (the mounted boot partition),
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

	result.SeedFiles = seedFiles(bootDir)

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
		log("%s found on the boot partition; gosd-init never parses or executes it — edit the files in %s/ to configure this device instead", firstrunFile, configtree.Dir)
	}

	return result
}

// seedFiles lists which of cloud-init's NoCloud seed files this card
// actually carries. meta-data is included even though nothing ever reads it
// (it holds only cloud-init's own instance-id bookkeeping): the seed is
// consumed as a whole, and leaving a third of it behind would leave a card
// that still looks provisioned to anything else that reads it.
func seedFiles(bootDir string) []string {
	var found []string
	for _, name := range []string{userDataFile, networkConfigFile, metaDataFile} {
		if _, err := os.Stat(filepath.Join(bootDir, name)); err == nil {
			found = append(found, name)
		}
	}
	return found
}

// DeleteSeed removes the named cloud-init seed files from bootDir. It is
// how a seed is consumed: the caller deletes it, makes that deletion
// durable, and only then writes what the seed asked for into the config
// tree. A power cut in the gap loses the wizard's answers — which can be
// given again by flashing again — where the reverse order would leave a
// seed that silently overwrites every later hand-edit, on every boot, for
// the life of the card (locked, epic gosd-rw6n).
//
// A file that has already gone is not an error: this runs on the first boot
// after a flash, and racing the very thing it is trying to remove is not
// worth failing over.
func DeleteSeed(bootDir string, names []string) error {
	var errs []error
	for _, name := range names {
		if err := os.Remove(filepath.Join(bootDir, name)); err != nil && !os.IsNotExist(err) {
			errs = append(errs, fmt.Errorf("deleting cloud-init's %s: %w", name, err))
		}
	}
	return errors.Join(errs...)
}

// MaxSeedBytes caps how much of a cloud-init seed file this package will
// read. A seed is a wizard's answers: the user-data and network-config
// files Raspberry Pi Imager 2.0.10 actually writes are a few hundred bytes
// each (see the captured fixtures in testdata), and gosd-init consumes a
// hostname and a WiFi network from between them. This leaves three orders
// of magnitude of room for a hand-written seed carrying cloud-init fields
// gosd ignores, and still bounds what the parse below can cost.
//
// It has to be bounded because of what happens after the read. A seed is a
// file on the FAT boot partition, so anyone holding the card writes one of
// any size, and yaml.v3 builds a Node tree that outweighs its input by
// roughly forty times — a few megabytes of YAML becomes hundreds of
// megabytes of nodes. gosd-init is PID 1 on a board whose entire root
// filesystem is RAM and which may have 512 MB of it in total; Linux will
// not kill init to reclaim memory, so exhausting it is a kernel panic and,
// since the file is still there on the next boot, a permanent one.
const MaxSeedBytes = 256 * 1024

// readOptional reads path, treating a missing file as a silent, expected
// case (ok=false, no log) and any other read error as worth surfacing
// (still ok=false, but logged) since it means a file is present but
// somehow inaccessible.
//
// Anything that isn't an ordinary file of at most MaxSeedBytes is refused
// before it is opened, not after: opening a named pipe blocks until
// something writes to it, which in PID 1 is a device that never finishes
// booting and never says why.
func readOptional(path, name string, log func(format string, args ...any)) ([]byte, bool) {
	info, err := os.Lstat(path)
	if err != nil {
		if !os.IsNotExist(err) {
			log("reading %s failed, ignoring it: %v", name, err)
		}
		return nil, false
	}
	if !info.Mode().IsRegular() {
		log("ignoring %s: cloud-init provisioning has to be an ordinary file", name)
		return nil, false
	}
	if info.Size() > MaxSeedBytes {
		log("ignoring %s: it holds %d bytes, far more than any cloud-init seed (%d at most)", name, info.Size(), MaxSeedBytes)
		return nil, false
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			log("reading %s failed, ignoring it: %v", name, err)
		}
		return nil, false
	}
	return data, true
}
