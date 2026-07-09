// Package initcfg owns the schema gosd-init reads at boot: the config.json
// baked onto every image by the gosd CLI, and the gosd.* kernel command-line
// parameters. Both are pure data formats with no syscall dependencies, so
// this package has no build tags and is fully unit-testable on any OS.
//
// Later beans (gosd.toml parsing, provisioning-file consumption) are
// expected to import this package for the Config type rather than defining
// their own.
package initcfg

import (
	"encoding/json"
	"fmt"
)

// Config is the schema of /etc/gosd/config.json, baked into every image at
// build time by the gosd CLI.
type Config struct {
	Board    string `json:"board"`
	Hostname string `json:"hostname"`
	Wifi     Wifi   `json:"wifi"`

	// NTPServers is the ordered list of NTP servers gosd-init's timesync
	// package queries. Optional: a nil/empty slice (including every
	// config.json baked before this field existed) means "use
	// timesync.DefaultServers" (pool.ntp.org); this package only owns the
	// schema, not that default, to keep it free of other packages'
	// constants.
	NTPServers []string `json:"ntpServers,omitempty"`

	// Env holds developer-set default app environment variables, baked in
	// at build time (gosd build --env). It's the lowest-precedence layer
	// in gosd.toml [env]'s locked merge: a hand-edited gosd.toml [env]
	// entry overrides the same key here. Optional: omitted entirely for
	// every config.json baked before this field existed.
	Env map[string]string `json:"env,omitempty"`
}

// Wifi holds the baked-in WPA2-PSK or open network credentials. Both fields
// empty means no WiFi is configured.
type Wifi struct {
	SSID       string `json:"ssid"`
	Passphrase string `json:"passphrase"`
}

// ParseConfig parses config.json contents into a Config. Missing data (a nil
// or empty slice, as when the file doesn't exist on disk) yields a zero
// Config rather than an error, since every field is optional. Malformed JSON
// is reported as an actionable error rather than crashing the caller.
func ParseConfig(data []byte) (Config, error) {
	if len(data) == 0 {
		return Config{}, nil
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("config.json is not valid JSON: %w", err)
	}
	return cfg, nil
}
