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
	"time"
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

	// DataExpand marks an image built with --data-size=expand: it ships
	// with no GOSD-DATA partition, and gosd-init creates and formats one
	// filling the rest of the card on first boot (see
	// cmd/gosd-init/internal/dataexpand). Optional: absent — including
	// every config.json baked before this field existed — means no
	// expansion, exactly like --data-size=0.
	DataExpand bool `json:"dataExpand,omitempty"`

	// Identity is a content-derived digest of this build's boot payload,
	// baked in by gosd build (see ComputeIdentity's docstring for the
	// exact recipe, and internal/pipeline for where it's computed).
	// Identical rebuilds from identical inputs produce the identical
	// Identity — it is never a timestamp or a random id — which is what
	// makes it usable both for upgrade-skew detection (does the running
	// image match what a provisioning snapshot was taken from?) and for
	// a future self-update's "am I already running this?" check.
	// Optional: empty for every config.json baked before this field
	// existed; callers must treat that as "unknown, not comparable"
	// rather than as a mismatch (see ShortIdentity).
	Identity string `json:"identity,omitempty"`

	// BuildTimestamp is when gosd build assembled this image (UTC,
	// RFC3339Nano), baked in by the CLI. It's gosd-init's timesync
	// package's clock floor (see BuildTime and gosd-0esw): neither board
	// has a battery-backed RTC, so the clock starts every boot near the
	// Unix epoch, and an SNTP result reporting a time before the image
	// was even built can only be wrong — a forged or badly misbehaving
	// server, never a legitimate one.
	//
	// Deliberately excluded from ComputeIdentity's payload without any
	// special-casing: that function's docstring already excludes
	// config.json in its entirety (not just Identity), so a value that
	// changes on every rebuild lives here safely — it never touches
	// build reproducibility.
	//
	// Optional: empty for every config.json baked before this field
	// existed; BuildTime returns the zero time.Time in that case, which
	// callers must treat as "no floor available," not as "before the
	// epoch."
	BuildTimestamp string `json:"buildTimestamp,omitempty"`
}

// BuildTime parses BuildTimestamp, returning the zero time.Time if it's
// empty or malformed (e.g. a config.json baked before this field
// existed) — callers must treat that as "no floor available," exactly
// how ShortIdentity treats an empty Identity as "unknown, not
// comparable."
func (c Config) BuildTime() time.Time {
	if c.BuildTimestamp == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, c.BuildTimestamp)
	if err != nil {
		return time.Time{}
	}
	return t
}

// shortIdentityLen is how many leading hex characters of Identity
// ShortIdentity keeps: enough to tell builds apart at a glance in a boot
// log line, without spelling out the full SHA-256.
const shortIdentityLen = 12

// ShortIdentity returns a truncated, human-scannable form of Identity, for
// logging (e.g. gosd-init's boot-time "[gosd] image identity: <short
// digest>" line). Empty when Identity is empty, e.g. an image built before
// this field existed.
func (c Config) ShortIdentity() string {
	if len(c.Identity) <= shortIdentityLen {
		return c.Identity
	}
	return c.Identity[:shortIdentityLen]
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
