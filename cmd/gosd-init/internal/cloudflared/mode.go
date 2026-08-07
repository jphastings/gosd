package cloudflared

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/jphastings/gosd/internal/gosdtoml"
)

// resolvedMode is the pure result of resolveMode: either run is true and
// the remaining fields hold everything writeRuntimeFiles needs to render
// credentials.json and config.yml, or run is false and log (if non-empty)
// is the single actionable line Run should log before returning. A false
// run with an empty log is the ordinary "nothing configured, nothing
// baked" case, which needs no log line at all.
type resolvedMode struct {
	run bool
	log string

	accountTag, tunnelSecret, tunnelID string
	hostname                           string
	port                               int
}

// resolveMode decides whether cloudflared should run at all, and validates
// everything about [ingress.cloudflared] that can be checked without
// touching the network, the clock, or the filesystem — deliberately pure so
// mode_test.go can assert on it directly. gosd.toml is only ever read once,
// at boot (nothing here or in Run self-heals a bad value later), so every
// misconfiguration this finds produces exactly one actionable log line and
// a run=false result; the failure modes below are the bean's locked list.
func resolveMode(cfg gosdtoml.IngressCloudflared, baked bool) resolvedMode {
	configured := cfg.Configured()

	switch {
	case !configured && !baked:
		// Nothing declared, nothing baked: the overwhelmingly common case
		// for a device with no ingress. Not worth a log line at all.
		return resolvedMode{}
	case !configured && baked:
		return resolvedMode{log: "cloudflared: binary is baked into this image, but [ingress.cloudflared] isn't configured in gosd.toml; nothing to do"}
	case configured && !baked:
		return resolvedMode{log: "cloudflared: [ingress.cloudflared] is configured in gosd.toml, but this image wasn't built with --ingress cloudflared; rebuild with that flag to bake the binary in"}
	}

	var missing []string
	if cfg.Token == "" {
		missing = append(missing, "token")
	}
	if cfg.Hostname == "" {
		missing = append(missing, "hostname")
	}
	if cfg.Port == 0 {
		missing = append(missing, "port")
	}
	if len(missing) > 0 {
		if cfg.Token != "" && cfg.Hostname == "" && cfg.Port == 0 {
			return resolvedMode{log: "cloudflared: [ingress.cloudflared] has a token but no hostname/port; remote mode not supported yet — add hostname and port to gosd.toml to run it locally-managed"}
		}
		return resolvedMode{log: fmt.Sprintf("cloudflared: [ingress.cloudflared] is missing required key(s): %s", strings.Join(missing, ", "))}
	}

	tok, err := decodeToken(cfg.Token)
	if err != nil {
		return resolvedMode{log: fmt.Sprintf(
			"cloudflared: [ingress.cloudflared] token is not a valid Cloudflare Tunnel token (%v); generate a fresh one with `cloudflared tunnel token <name>` or copy it from the Cloudflare dashboard — if this token used to work, a gosd update may be needed to support a new token format",
			err,
		)}
	}

	if !validHostname(cfg.Hostname) {
		return resolvedMode{log: fmt.Sprintf("cloudflared: [ingress.cloudflared] hostname %q is not a valid fully-qualified hostname", cfg.Hostname)}
	}
	if cfg.Port < 1 || cfg.Port > 65535 {
		return resolvedMode{log: fmt.Sprintf("cloudflared: [ingress.cloudflared] port %d is out of range (must be 1-65535)", cfg.Port)}
	}

	return resolvedMode{
		run:          true,
		accountTag:   tok.AccountTag,
		tunnelSecret: tok.TunnelSecret,
		tunnelID:     tok.TunnelID,
		hostname:     cfg.Hostname,
		port:         cfg.Port,
	}
}

// tunnelToken mirrors the JSON payload base64-encoded inside a `cloudflared
// tunnel token <name>` value (or the one copied from the dashboard): an
// account tag, tunnel secret, and tunnel ID — exactly the three fields
// cloudflared's own credentials-file JSON needs (see credentialsJSON). The
// format is undocumented but stable since 2021 (verified against real
// release assets during this epic's planning); unknown extra fields are
// tolerated since encoding/json ignores them by default, and a future
// cloudflared release adding one must not break decoding.
type tunnelToken struct {
	AccountTag   string `json:"a"`
	TunnelSecret string `json:"s"`
	TunnelID     string `json:"t"`
}

// decodeToken decodes a tunnel token into its three fields, trying every
// base64 alphabet cloudflared might have used (standard and URL-safe,
// padded and raw) since the token's own format doesn't say which. It fails
// only once every alphabet has failed to decode, to fail to parse as JSON,
// or to produce all three required fields.
func decodeToken(raw string) (tunnelToken, error) {
	var lastErr error
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.URLEncoding, base64.RawStdEncoding, base64.RawURLEncoding} {
		data, err := enc.DecodeString(raw)
		if err != nil {
			lastErr = err
			continue
		}
		var tok tunnelToken
		if err := json.Unmarshal(data, &tok); err != nil {
			lastErr = err
			continue
		}
		if tok.AccountTag == "" || tok.TunnelSecret == "" || tok.TunnelID == "" {
			lastErr = fmt.Errorf("decoded token is missing one of the required a/s/t fields")
			continue
		}
		return tok, nil
	}
	return tunnelToken{}, lastErr
}

// fqdnPattern is a strict fully-qualified-hostname shape: labels of
// alphanumerics and internal hyphens (1-63 chars each), at least two
// labels, ASCII only. This is deliberately stricter than every hostname DNS
// itself would accept — it exists solely so a hand-edited gosd.toml
// hostname can never contain a character (':', newline, '#', ...) that
// would let it break out of config.yml's line (see configYAML): rejecting
// it here, with an actionable error, is preferable to gosd-init generating
// a config.yml that isn't the one the user asked for.
var fqdnPattern = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,63}$`)

// validHostname reports whether h is safe to embed, unescaped, in
// config.yml's ingress rule.
func validHostname(h string) bool {
	return len(h) <= 253 && fqdnPattern.MatchString(h)
}

// credentialsJSON renders /run/gosd/cloudflared/credentials.json: the same
// three fields the tunnel token decodes to, under the field names
// cloudflared's own `tunnel token --cred-file` output uses (the epic's
// locked "the token IS the credentials triple" decision).
func credentialsJSON(m resolvedMode) []byte {
	data, err := json.Marshal(struct {
		AccountTag   string `json:"AccountTag"`
		TunnelSecret string `json:"TunnelSecret"`
		TunnelID     string `json:"TunnelID"`
	}{m.accountTag, m.tunnelSecret, m.tunnelID})
	if err != nil {
		// Marshaling three plain strings cannot fail.
		panic(err)
	}
	return data
}

// configYAML renders /run/gosd/cloudflared/config.yml by hand, string
// formatting rather than pulling in a YAML library for three lines of
// output (locked decision). This is injection-safe only because
// resolveMode has already validated m.hostname (validHostname) and m.port
// (range-checked) before a resolvedMode with run=true is ever produced;
// m.tunnelID comes from the operator's own trusted tunnel token, not from
// an untrusted third party. The trailing http_status:404 catch-all rule is
// mandatory: cloudflared refuses to start on a config with no catch-all.
func configYAML(m resolvedMode) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "tunnel: %s\n", m.tunnelID)
	fmt.Fprintf(&b, "credentials-file: %s\n", CredentialsPath)
	b.WriteString("ingress:\n")
	fmt.Fprintf(&b, "  - hostname: %s\n", m.hostname)
	fmt.Fprintf(&b, "    service: http://localhost:%d\n", m.port)
	b.WriteString("  - service: http_status:404\n")
	return []byte(b.String())
}
