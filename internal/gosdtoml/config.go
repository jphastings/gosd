// Package gosdtoml implements gosd.toml: the optional, hand-editable
// configuration file the gosd CLI writes to the root of every image's FAT
// boot partition, and gosd-init reads back after mounting that partition.
// Its schema (v1, locked) is deliberately tiny — a top-level hostname and a
// [wifi] table with ssid/passphrase, everything optional — so that a
// non-technical user can safely edit it in any text editor. See bean
// gosd-tds2.
package gosdtoml

import (
	"bytes"
	"fmt"

	"github.com/BurntSushi/toml"
)

// Config is the schema of gosd.toml, schema v1 (locked): every field is
// optional, and a missing file parses the same as an empty one.
type Config struct {
	Hostname string `toml:"hostname"`
	Wifi     Wifi   `toml:"wifi"`
}

// Wifi holds the WPA2-PSK or open network a user has hand-entered into
// gosd.toml. Both fields empty means no WiFi override is configured.
type Wifi struct {
	SSID       string `toml:"ssid"`
	Passphrase string `toml:"passphrase"`
}

// Parse parses gosd.toml's contents into a Config. Missing data (nil or
// empty, as when the file doesn't exist on the boot partition) yields a
// zero Config and no error, since every field is optional.
//
// Malformed TOML — a typo a hand-editing user is bound to make eventually —
// is reported as an error rather than causing a panic. gosd-init must never
// fail to boot over it: callers are expected to log Parse's error and fall
// back to the zero Config (or another source's values, per gosd.toml's
// documented precedence) rather than propagate the error further.
func Parse(data []byte) (Config, error) {
	if len(data) == 0 {
		return Config{}, nil
	}

	var cfg Config
	if _, err := toml.NewDecoder(bytes.NewReader(data)).Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("gosd.toml is not valid TOML: %w", err)
	}
	return cfg, nil
}
