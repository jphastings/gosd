package cloudflared

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jphastings/gosd/internal/gosdtoml"
)

const (
	testAccountTag   = "account-0123456789abcdef"
	testTunnelSecret = "super-secret-value-do-not-log-me"
	testTunnelID     = "tunnel-fedcba9876543210"
)

func tokenJSON(t *testing.T, extra map[string]any) []byte {
	t.Helper()
	m := map[string]any{"a": testAccountTag, "s": testTunnelSecret, "t": testTunnelID}
	for k, v := range extra {
		m[k] = v
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshaling test token JSON: %v", err)
	}
	return data
}

func TestDecodeTokenAcceptsEveryBase64Alphabet(t *testing.T) {
	raw := tokenJSON(t, nil)
	encs := map[string]*base64.Encoding{
		"std":     base64.StdEncoding,
		"url":     base64.URLEncoding,
		"raw-std": base64.RawStdEncoding,
		"raw-url": base64.RawURLEncoding,
	}
	for name, enc := range encs {
		t.Run(name, func(t *testing.T) {
			tok, err := decodeToken(enc.EncodeToString(raw))
			if err != nil {
				t.Fatalf("decodeToken: %v", err)
			}
			if tok.AccountTag != testAccountTag || tok.TunnelSecret != testTunnelSecret || tok.TunnelID != testTunnelID {
				t.Fatalf("decodeToken = %+v, want a=%s s=%s t=%s", tok, testAccountTag, testTunnelSecret, testTunnelID)
			}
		})
	}
}

func TestDecodeTokenToleratesUnknownFields(t *testing.T) {
	raw := tokenJSON(t, map[string]any{"future_field": "some-new-thing"})
	tok, err := decodeToken(base64.StdEncoding.EncodeToString(raw))
	if err != nil {
		t.Fatalf("decodeToken: %v", err)
	}
	if tok.TunnelID != testTunnelID {
		t.Fatalf("decodeToken = %+v, want TunnelID %s", tok, testTunnelID)
	}
}

func TestDecodeTokenFailsOnMissingField(t *testing.T) {
	m := map[string]any{"a": testAccountTag, "s": testTunnelSecret} // no "t"
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if _, err := decodeToken(base64.StdEncoding.EncodeToString(data)); err == nil {
		t.Fatal("decodeToken succeeded on a token missing the required t field, want an error")
	}
}

func TestDecodeTokenFailsOnGarbage(t *testing.T) {
	if _, err := decodeToken("this is not a token at all!!"); err == nil {
		t.Fatal("decodeToken succeeded on garbage input, want an error")
	}
}

func TestValidHostname(t *testing.T) {
	tests := []struct {
		name string
		host string
		want bool
	}{
		{"simple fqdn", "app.example.com", true},
		{"deep subdomain", "a.b.c.example.com", true},
		{"hyphenated label", "my-app.example.com", true},
		{"single label", "localhost", false},
		{"empty", "", false},
		{"leading hyphen", "-bad.example.com", false},
		{"trailing hyphen", "bad-.example.com", false},
		{"trailing dot", "app.example.com.", false},
		{"contains colon", "app.example.com:8080", false},
		{"contains space", "app example.com", false},
		{"contains newline", "app.example.com\ningress:\n  - hostname: evil", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validHostname(tt.host); got != tt.want {
				t.Errorf("validHostname(%q) = %v, want %v", tt.host, got, tt.want)
			}
		})
	}
}

// mustToken returns a valid, encoded tunnel token for use in resolveMode
// tests, encapsulating tokenJSON's harmless failure path.
func mustToken(t *testing.T) string {
	t.Helper()
	return base64.StdEncoding.EncodeToString(tokenJSON(t, nil))
}

func TestResolveModeFailureModes(t *testing.T) {
	validToken := func(t *testing.T) string { return mustToken(t) }

	tests := []struct {
		name       string
		cfg        func(t *testing.T) gosdtoml.IngressCloudflared
		baked      bool
		wantRun    bool
		wantLog    string // substring; "" means "no log line at all"
		notWantLog string // substring that must never appear (e.g. the secret)
	}{
		{
			name:    "unconfigured and not baked: silent no-op",
			cfg:     func(t *testing.T) gosdtoml.IngressCloudflared { return gosdtoml.IngressCloudflared{} },
			baked:   false,
			wantRun: false,
			wantLog: "",
		},
		{
			name:    "unconfigured but baked: one quiet line",
			cfg:     func(t *testing.T) gosdtoml.IngressCloudflared { return gosdtoml.IngressCloudflared{} },
			baked:   true,
			wantRun: false,
			wantLog: "nothing to do",
		},
		{
			name: "configured but not baked: points at --ingress",
			cfg: func(t *testing.T) gosdtoml.IngressCloudflared {
				return gosdtoml.IngressCloudflared{Token: validToken(t), Hostname: "app.example.com", Port: 8080}
			},
			baked:   false,
			wantRun: false,
			wantLog: "--ingress cloudflared",
		},
		{
			name: "token only: remote mode not supported yet",
			cfg: func(t *testing.T) gosdtoml.IngressCloudflared {
				return gosdtoml.IngressCloudflared{Token: validToken(t)}
			},
			baked:      true,
			wantRun:    false,
			wantLog:    "remote mode not supported yet",
			notWantLog: testTunnelSecret,
		},
		{
			name: "missing hostname",
			cfg: func(t *testing.T) gosdtoml.IngressCloudflared {
				return gosdtoml.IngressCloudflared{Token: validToken(t), Port: 8080}
			},
			baked:      true,
			wantRun:    false,
			wantLog:    "missing required key(s): hostname",
			notWantLog: testTunnelSecret,
		},
		{
			name: "missing port",
			cfg: func(t *testing.T) gosdtoml.IngressCloudflared {
				return gosdtoml.IngressCloudflared{Token: validToken(t), Hostname: "app.example.com"}
			},
			baked:   true,
			wantRun: false,
			wantLog: "missing required key(s): port",
		},
		{
			name: "missing token",
			cfg: func(t *testing.T) gosdtoml.IngressCloudflared {
				return gosdtoml.IngressCloudflared{Hostname: "app.example.com", Port: 8080}
			},
			baked:   true,
			wantRun: false,
			wantLog: "missing required key(s): token",
		},
		{
			name: "bad token",
			cfg: func(t *testing.T) gosdtoml.IngressCloudflared {
				return gosdtoml.IngressCloudflared{Token: "not-a-real-token", Hostname: "app.example.com", Port: 8080}
			},
			baked:   true,
			wantRun: false,
			wantLog: "not a valid Cloudflare Tunnel token",
		},
		{
			name: "invalid hostname format",
			cfg: func(t *testing.T) gosdtoml.IngressCloudflared {
				return gosdtoml.IngressCloudflared{Token: validToken(t), Hostname: "not a hostname", Port: 8080}
			},
			baked:      true,
			wantRun:    false,
			wantLog:    "not a valid fully-qualified hostname",
			notWantLog: testTunnelSecret,
		},
		{
			name: "port out of range",
			cfg: func(t *testing.T) gosdtoml.IngressCloudflared {
				return gosdtoml.IngressCloudflared{Token: validToken(t), Hostname: "app.example.com", Port: 70000}
			},
			baked:   true,
			wantRun: false,
			wantLog: "out of range",
		},
		{
			name: "fully valid: runs, no log line",
			cfg: func(t *testing.T) gosdtoml.IngressCloudflared {
				return gosdtoml.IngressCloudflared{Token: validToken(t), Hostname: "app.example.com", Port: 8080}
			},
			baked:   true,
			wantRun: true,
			wantLog: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := resolveMode(tt.cfg(t), tt.baked)
			if m.run != tt.wantRun {
				t.Errorf("run = %v, want %v (log: %q)", m.run, tt.wantRun, m.log)
			}
			if tt.wantLog == "" {
				if m.log != "" {
					t.Errorf("log = %q, want no log line at all", m.log)
				}
			} else if !strings.Contains(m.log, tt.wantLog) {
				t.Errorf("log = %q, want it to contain %q", m.log, tt.wantLog)
			}
			if tt.notWantLog != "" && strings.Contains(m.log, tt.notWantLog) {
				t.Errorf("log = %q leaked the secret %q", m.log, tt.notWantLog)
			}
		})
	}
}

func TestResolveModeValidConfigPopulatesDecodedFields(t *testing.T) {
	cfg := gosdtoml.IngressCloudflared{Token: mustToken(t), Hostname: "app.example.com", Port: 8080}
	m := resolveMode(cfg, true)

	if !m.run {
		t.Fatalf("run = false, want true (log: %q)", m.log)
	}
	if m.accountTag != testAccountTag || m.tunnelSecret != testTunnelSecret || m.tunnelID != testTunnelID {
		t.Errorf("decoded fields = %+v, want a=%s s=%s t=%s", m, testAccountTag, testTunnelSecret, testTunnelID)
	}
	if m.hostname != "app.example.com" || m.port != 8080 {
		t.Errorf("hostname/port = %s/%d, want app.example.com/8080", m.hostname, m.port)
	}
}

func TestCredentialsJSONGolden(t *testing.T) {
	m := resolvedMode{accountTag: testAccountTag, tunnelSecret: testTunnelSecret, tunnelID: testTunnelID}
	got := string(credentialsJSON(m))
	want := `{"AccountTag":"` + testAccountTag + `","TunnelSecret":"` + testTunnelSecret + `","TunnelID":"` + testTunnelID + `"}`
	if got != want {
		t.Errorf("credentialsJSON =\n%s\nwant\n%s", got, want)
	}
}

func TestConfigYAMLGolden(t *testing.T) {
	m := resolvedMode{tunnelID: testTunnelID, hostname: "app.example.com", port: 8080}
	got := string(configYAML(m))
	want := "tunnel: " + testTunnelID + "\n" +
		"credentials-file: /run/gosd/cloudflared/credentials.json\n" +
		"ingress:\n" +
		"  - hostname: app.example.com\n" +
		"    service: http://localhost:8080\n" +
		"  - service: http_status:404\n"
	if got != want {
		t.Errorf("configYAML =\n%s\nwant\n%s", got, want)
	}
}
