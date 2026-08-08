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
	"strconv"

	"github.com/BurntSushi/toml"
)

// Config is the schema of gosd.toml, schema v1 (locked): every field is
// optional, and a missing file parses the same as an empty one.
type Config struct {
	Hostname string            `toml:"hostname"`
	Wifi     Wifi              `toml:"wifi"`
	Env      map[string]string `toml:"env"`
	Ingress  Ingress           `toml:"ingress"`

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

// Ingress is a table of the internet-facing tunnels this device declares.
// It's a table of tables — not each provider inlined directly under
// [ingress] — so each provider gets its own sibling table
// ([ingress.<provider>]) without a schema break; TailscaleFunnel is the
// second provider to use that room, after Cloudflared.
type Ingress struct {
	Cloudflared     IngressCloudflared     `toml:"cloudflared"`
	TailscaleFunnel IngressTailscaleFunnel `toml:"tailscale-funnel"`
}

// IngressCloudflared is a locally-managed Cloudflare Tunnel declaration:
// the tunnel token (from `cloudflared tunnel token <name>` or the
// dashboard), the public hostname it should answer for, and the local port
// its traffic is forwarded to. All three are required for the tunnel to
// actually run; Parse only shapes the values (see coerceIngress) and never
// checks that — FQDN shape, port range, which keys are even present — is
// semantic validation that belongs to the future cloudflared runtime
// module, once it exists to be validated against (validHostname's
// precedent: gosd.toml is parsed long before that).
type IngressCloudflared struct {
	Token    string `toml:"token"`
	Hostname string `toml:"hostname"`
	Port     int    `toml:"port"`
}

// Configured reports whether any field has been set, i.e. whether this
// gosd.toml declares (or attempts to declare) a Cloudflare Tunnel at all.
func (c IngressCloudflared) Configured() bool {
	return c.Token != "" || c.Hostname != "" || c.Port != 0
}

// IngressTailscaleFunnel is a locally-managed Tailscale Funnel declaration:
// a tagged, reusable auth key from the tailnet's admin console (tagging a
// key disables that node's key expiry, so a shipped device never gets
// locked out — see epic gosd-65uy decision 4), the local app port to
// expose, and two optional fields — the public hostname (defaults to the
// device's own hostname) and which of Tailscale's three funnel ports to
// use (defaults to 443). The auth key is needed only for this device's
// first registration: tsnet ignores it once local state already exists, so
// it's safe to remove from gosd.toml afterwards. As with IngressCloudflared,
// Parse only shapes these values (see coerceIngress) and never checks them
// — port range, funnel_port's {443, 8443, 10000} set membership, hostname
// defaulting — that's semantic validation for the future tsfunnel runtime
// module (epic gosd-65uy), once it exists to validate against
// (validHostname's precedent: gosd.toml is parsed long before that).
type IngressTailscaleFunnel struct {
	Authkey    string `toml:"authkey"`
	Hostname   string `toml:"hostname"`
	Port       int    `toml:"port"`
	FunnelPort int    `toml:"funnel_port"`
}

// Configured reports whether any field has been set, i.e. whether this
// gosd.toml declares (or attempts to declare) a Tailscale Funnel at all.
func (t IngressTailscaleFunnel) Configured() bool {
	return t.Authkey != "" || t.Hostname != "" || t.Port != 0 || t.FunnelPort != 0
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
	Ingress   rawIngress     `toml:"ingress"`
	DataFlush any            `toml:"data_flush"`
}

// rawIngress mirrors Ingress the way rawConfig mirrors Config: each
// [ingress.<provider>] field is decoded into `any` so coerceIngress can
// apply its own, more forgiving typing rules instead of letting a bare
// scalar fail the whole parse.
type rawIngress struct {
	Cloudflared     rawIngressCloudflared     `toml:"cloudflared"`
	TailscaleFunnel rawIngressTailscaleFunnel `toml:"tailscale-funnel"`
}

type rawIngressCloudflared struct {
	Token    any `toml:"token"`
	Hostname any `toml:"hostname"`
	Port     any `toml:"port"`
}

type rawIngressTailscaleFunnel struct {
	Authkey    any `toml:"authkey"`
	Hostname   any `toml:"hostname"`
	Port       any `toml:"port"`
	FunnelPort any `toml:"funnel_port"`
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
// (bean gosd-9m1k). Each [ingress.<provider>] table ([ingress.cloudflared],
// [ingress.tailscale-funnel]) gets the same [env]/data_flush treatment
// field by field (see coerceIngress), except its warnings never echo the
// coerced value at all, even for hostname and port: cloudflared's token and
// tailscale-funnel's authkey are secrets, and each whole table follows one
// discipline rather than special-casing just the one field that needs it
// (mergeUserEnv precedent).
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
	ingress, ingressWarnings := coerceIngress(raw.Ingress)
	warnings = append(warnings, ingressWarnings...)
	cfg := Config{
		Hostname:  raw.Hostname,
		Wifi:      raw.Wifi,
		Env:       env,
		Ingress:   ingress,
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

// coerceIngress turns the raw [ingress.*] tables into an Ingress, one
// provider table at a time (see coerceIngressCloudflared and
// coerceIngressTailscaleFunnel) — each field by field, following the same
// coercion rules: strings are meant to be quoted, so a bare scalar is
// coerced to text with a warning, the same leniency [env] applies; ints are
// meant to be bare integers, so a quoted all-digit string is also accepted
// with a warning, data_flush's mirror-image leniency (see coerceDataFlush).
// Every warning names only the key, never the value — cloudflared's token
// and tailscale-funnel's authkey are secrets, and every other field follows
// the same discipline for consistency (mergeUserEnv precedent) rather than
// special-casing just the fields that need it. Warning order is fixed
// (cloudflared's fields, in struct order, then tailscale-funnel's) rather
// than sorted, since each table is a fixed shape, not a map with
// unpredictable iteration order like [env] — Parse's overall output is
// still deterministic.
func coerceIngress(raw rawIngress) (Ingress, []string) {
	cloudflared, cloudflaredWarnings := coerceIngressCloudflared(raw.Cloudflared)
	tailscaleFunnel, tailscaleFunnelWarnings := coerceIngressTailscaleFunnel(raw.TailscaleFunnel)

	warnings := append(cloudflaredWarnings, tailscaleFunnelWarnings...)
	return Ingress{Cloudflared: cloudflared, TailscaleFunnel: tailscaleFunnel}, warnings
}

// coerceIngressCloudflared coerces [ingress.cloudflared]'s three fields,
// field by field, in struct order (token, hostname, port).
func coerceIngressCloudflared(table rawIngressCloudflared) (IngressCloudflared, []string) {
	var warnings []string

	token, warning := coerceIngressString("cloudflared", "token", table.Token)
	if warning != "" {
		warnings = append(warnings, warning)
	}
	hostname, warning := coerceIngressString("cloudflared", "hostname", table.Hostname)
	if warning != "" {
		warnings = append(warnings, warning)
	}
	port, warning := coerceIngressPort("cloudflared", "port", table.Port)
	if warning != "" {
		warnings = append(warnings, warning)
	}

	return IngressCloudflared{Token: token, Hostname: hostname, Port: port}, warnings
}

// coerceIngressTailscaleFunnel coerces [ingress.tailscale-funnel]'s four
// fields, field by field, in struct order (authkey, hostname, port,
// funnel_port) — gosd-7upw's coercion style (coerceIngressCloudflared),
// mirrored exactly: authkey is this table's secret, the same role token
// plays for cloudflared, so it gets the same never-echoed treatment via
// coerceIngressString.
func coerceIngressTailscaleFunnel(table rawIngressTailscaleFunnel) (IngressTailscaleFunnel, []string) {
	var warnings []string

	authkey, warning := coerceIngressString("tailscale-funnel", "authkey", table.Authkey)
	if warning != "" {
		warnings = append(warnings, warning)
	}
	hostname, warning := coerceIngressString("tailscale-funnel", "hostname", table.Hostname)
	if warning != "" {
		warnings = append(warnings, warning)
	}
	port, warning := coerceIngressPort("tailscale-funnel", "port", table.Port)
	if warning != "" {
		warnings = append(warnings, warning)
	}
	funnelPort, warning := coerceIngressPort("tailscale-funnel", "funnel_port", table.FunnelPort)
	if warning != "" {
		warnings = append(warnings, warning)
	}

	return IngressTailscaleFunnel{Authkey: authkey, Hostname: hostname, Port: port, FunnelPort: funnelPort}, warnings
}

// coerceIngressString coerces one of an [ingress.<provider>] table's string
// fields (e.g. cloudflared's token or hostname, tailscale-funnel's authkey
// or hostname): a bare scalar is coerced to its string form, a non-scalar
// is dropped, and — unlike coerceEnv's equivalent path — neither warning
// ever shows the value, coerced or otherwise, so a hand-edited secret can
// never leak into a log.
func coerceIngressString(table, key string, raw any) (string, string) {
	switch v := raw.(type) {
	case nil:
		return "", ""
	case string:
		return v, ""
	case int64, float64, bool:
		return fmt.Sprintf("%v", v), fmt.Sprintf(
			"gosd.toml [ingress.%s] %s is a bare %s, not a quoted string; using it as text — add quotes to silence this warning",
			table, key, tomlTypeName(v),
		)
	default:
		return "", fmt.Sprintf(
			"gosd.toml [ingress.%s] %s isn't a plain value (found %s); ignoring it",
			table, key, tomlTypeName(v),
		)
	}
}

// coerceIngressPort coerces one of an [ingress.<provider>] table's integer
// fields (e.g. cloudflared's port, tailscale-funnel's port or funnel_port):
// a bare TOML integer is used as-is (including out-of-range or negative
// values — range/set-membership is semantic validation, not Parse's job,
// see Ingress's docstring); a quoted, all-digit string ("8080") is accepted
// with a warning, the same leniency data_flush applies the other way
// around; anything else leaves the field unset (0) with a warning. As with
// the string fields, the value is never echoed.
func coerceIngressPort(table, key string, raw any) (int, string) {
	switch v := raw.(type) {
	case nil:
		return 0, ""
	case int64:
		return int(v), ""
	case string:
		if isAllDigits(v) {
			if port, err := strconv.Atoi(v); err == nil {
				return port, fmt.Sprintf(
					"gosd.toml [ingress.%s] %s is a quoted number, not a bare integer; using it — remove the quotes to silence this warning",
					table, key,
				)
			}
		}
		return 0, fmt.Sprintf("gosd.toml [ingress.%s] %s is not a whole number; ignoring it", table, key)
	default:
		return 0, fmt.Sprintf(
			"gosd.toml [ingress.%s] %s isn't a plain value (found %s); ignoring it",
			table, key, tomlTypeName(v),
		)
	}
}

// isAllDigits reports whether s is non-empty and every rune is an ASCII
// digit — the shape coerceIngressPort accepts for a quoted port number.
func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
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
