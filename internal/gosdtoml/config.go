// Package gosdtoml implements gosd.toml: the optional, hand-editable
// configuration file the gosd CLI writes to the root of every image's FAT
// boot partition, and gosd-init reads back after mounting that partition.
// Its schema (v1, locked) is deliberately tiny — a top-level hostname, a
// [wifi] table with ssid/passphrase, and an [env] table of app environment
// variables, everything optional — so that a non-technical user can safely
// edit it in any text editor. See bean gosd-tds2 (hostname/wifi) and
// gosd-9b5c ([env]).
package gosdtoml

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/BurntSushi/toml"
)

// Config is the schema of gosd.toml, schema v1 (locked): every field is
// optional, and a missing file parses the same as an empty one.
type Config struct {
	Hostname string            `toml:"hostname"`
	Wifi     Wifi              `toml:"wifi"`
	Env      map[string]string `toml:"env"`

	// DataFlush overrides config.json's baked vfat "flush" mount-option
	// default (gosd build --data-flush, see internal/initcfg.Config.
	// DataFlush) for this specific device. nil means "absent — use the
	// baked value", the same absent-means-inherit convention as every
	// other gosd.toml key; a hand-edited data_flush always wins, whichever
	// way it points (bean gosd-9m1k). Unlike Hostname/Wifi/Env, it isn't
	// restored by the provisioning snapshot across a reflash (see
	// cmd/gosd-init/internal/provsnapshot) — docs/runtime.md's "What does
	// not come back" already covers it as "anything outside
	// hostname/WiFi/[env]".
	DataFlush *bool
}

// Wifi holds the WPA2-PSK or open network a user has hand-entered into
// gosd.toml. Both fields empty means no WiFi override is configured.
type Wifi struct {
	SSID       string `toml:"ssid"`
	Passphrase string `toml:"passphrase"`
}

// rawConfig mirrors Config, except [env] is decoded into map[string]any
// rather than map[string]string. Decoding straight into map[string]string
// would make toml.Decode itself fail whenever a hand-editing user writes a
// bare scalar (PORT = 8080) instead of a quoted string — a type mismatch
// error from the TOML library, with no chance for us to warn-and-coerce
// instead. Going through map[string]any lets coerceEnv apply gosd.toml's
// own, more forgiving rules.
type rawConfig struct {
	Hostname  string         `toml:"hostname"`
	Wifi      Wifi           `toml:"wifi"`
	Env       map[string]any `toml:"env"`
	DataFlush any            `toml:"data_flush"`
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
//
// [env] gets its own, softer error handling on top of that: a bare scalar
// value (PORT = 8080) is coerced to its string form, and a non-scalar value
// (an array, an inline table, a datetime) is dropped — neither ever fails
// the parse of the rest of the file. Both are reported back as warnings for
// the caller to log (never silently), since gosd-init has no interactive
// surface to surface them any other way. data_flush gets the mirror-image
// leniency (see coerceDataFlush): it's meant to be written as a bare
// boolean, so a quoted "true"/"false" is coerced with a warning, and
// anything else is dropped (falling back to config.json's baked default)
// with a warning of its own — a malformed override must never stop boot
// (bean gosd-9m1k).
func Parse(data []byte) (Config, []string, error) {
	if len(data) == 0 {
		return Config{}, nil, nil
	}

	var raw rawConfig
	if _, err := toml.NewDecoder(bytes.NewReader(data)).Decode(&raw); err != nil {
		return Config{}, nil, fmt.Errorf("gosd.toml is not valid TOML: %w", err)
	}

	env, warnings := coerceEnv(raw.Env)
	dataFlush, dataFlushWarning := coerceDataFlush(raw.DataFlush)
	if dataFlushWarning != "" {
		warnings = append([]string{dataFlushWarning}, warnings...)
	}
	cfg := Config{
		Hostname:  raw.Hostname,
		Wifi:      raw.Wifi,
		Env:       env,
		DataFlush: dataFlush,
	}
	return cfg, warnings, nil
}

// coerceDataFlush turns the raw data_flush value into a *bool override, or
// nil ("absent — use config.json's baked default") plus a warning to log —
// [env]'s coercion leniency, mirrored the other way around: data_flush is
// meant to be written as a bare TOML boolean (data_flush = true), so that
// form is used as-is; a quoted "true"/"false" (an easy mistake, since [env]
// values must be quoted) is still honored, with a warning; anything else —
// a number, an array, a misspelled string — is dropped, keeping the baked
// default, so a malformed override can never stop boot or silently apply
// the wrong value (see gosd-9m1k).
func coerceDataFlush(raw any) (*bool, string) {
	switch v := raw.(type) {
	case nil:
		return nil, ""
	case bool:
		return &v, ""
	case string:
		if v == "true" || v == "false" {
			b := v == "true"
			return &b, fmt.Sprintf(
				"gosd.toml data_flush is a quoted %q, not a bare boolean; using %t — remove the quotes to silence this warning",
				v, b,
			)
		}
		return nil, fmt.Sprintf(
			"gosd.toml data_flush %q is not true or false; using the baked default",
			v,
		)
	default:
		return nil, fmt.Sprintf(
			"gosd.toml data_flush isn't a plain boolean (found %s); using the baked default",
			tomlTypeName(v),
		)
	}
}

// coerceEnv turns a raw, freely-typed [env] table into the quoted-strings-
// only map gosd-init and the rest of gosd deal in, per gosd.toml's locked
// [env] rules: strings pass through unchanged; a bare integer, float or
// bool is coerced to its canonical string form; anything else (array,
// inline table, datetime) is dropped. Coercions and drops each produce one
// warning, in sorted-key order so Parse's output is deterministic.
func coerceEnv(raw map[string]any) (map[string]string, []string) {
	if len(raw) == 0 {
		return nil, nil
	}

	keys := make([]string, 0, len(raw))
	for key := range raw {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	env := make(map[string]string, len(raw))
	var warnings []string
	for _, key := range keys {
		switch value := raw[key].(type) {
		case string:
			env[key] = value
		case int64, float64, bool:
			coerced := fmt.Sprintf("%v", value)
			env[key] = coerced
			warnings = append(warnings, fmt.Sprintf(
				"gosd.toml [env] %s is a bare %s, not a quoted string; using %q — add quotes to silence this warning",
				key, tomlTypeName(value), coerced,
			))
		default:
			warnings = append(warnings, fmt.Sprintf(
				"gosd.toml [env] %s isn't a plain value (found %s); ignoring it",
				key, tomlTypeName(value),
			))
		}
	}
	if len(env) == 0 {
		env = nil
	}
	return env, warnings
}

// tomlTypeName names the decoded Go type of a TOML value in the vocabulary
// a gosd.toml-editing user would recognise, for warning messages.
func tomlTypeName(value any) string {
	switch value.(type) {
	case int64:
		return "number"
	case float64:
		return "number"
	case bool:
		return "boolean"
	case []any:
		return "array"
	case map[string]any:
		return "table"
	default:
		return fmt.Sprintf("%T", value)
	}
}
