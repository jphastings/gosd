package gosdtoml

import (
	"reflect"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name         string
		data         string
		want         Config
		wantWarnings []string
		wantErr      bool
	}{
		{
			name: "full config",
			data: `
hostname = "my-device"

[wifi]
ssid = "home"
passphrase = "hunter2"
`,
			want: Config{
				Hostname: "my-device",
				Wifi:     Wifi{SSID: "home", Passphrase: "hunter2"},
			},
		},
		{
			name: "partial config leaves the rest zero",
			data: `hostname = "my-device"`,
			want: Config{Hostname: "my-device"},
		},
		{
			name: "only wifi, no hostname",
			data: `
[wifi]
ssid = "guest-net"
`,
			want: Config{Wifi: Wifi{SSID: "guest-net"}},
		},
		{
			name: "commented-out template parses as empty",
			data: `
# hostname = "my-device"
# [wifi]
# ssid = "MyHomeNetwork"
# passphrase = "MyWiFiPassword"
`,
			want: Config{},
		},
		{
			name: "missing file (empty data) is not an error",
			data: "",
			want: Config{},
		},
		{
			name:    "garbage input is reported, not panicked",
			data:    "this is not valid = = toml [[[",
			wantErr: true,
		},
		{
			name: "env values are quoted strings, passed through as-is",
			data: `
[env]
API_URL = "https://example.com"
LOG_LEVEL = "debug"
`,
			want: Config{Env: map[string]string{"API_URL": "https://example.com", "LOG_LEVEL": "debug"}},
		},
		{
			name: "missing [env] table leaves Env nil",
			data: `hostname = "my-device"`,
			want: Config{Hostname: "my-device"},
		},
		{
			name: "empty [env] table leaves Env nil",
			data: `
[env]
`,
			want: Config{},
		},
		{
			name: "bare scalars under [env] are coerced to their string form, with a warning each",
			data: `
[env]
PORT = 8080
RATIO = 1.5
DEBUG = true
`,
			want: Config{Env: map[string]string{"PORT": "8080", "RATIO": "1.5", "DEBUG": "true"}},
			wantWarnings: []string{
				`gosd.toml [env] DEBUG is a bare boolean, not a quoted string; using "true" — add quotes to silence this warning`,
				`gosd.toml [env] PORT is a bare number, not a quoted string; using "8080" — add quotes to silence this warning`,
				`gosd.toml [env] RATIO is a bare number, not a quoted string; using "1.5" — add quotes to silence this warning`,
			},
		},
		{
			name: "non-scalar values under [env] are skipped, with a warning each",
			data: `
[env]
KEEP = "yes"
LIST = [1, 2, 3]
TABLE = { x = 1 }
WHEN = 2026-07-08T00:00:00Z
`,
			want: Config{Env: map[string]string{"KEEP": "yes"}},
			wantWarnings: []string{
				`gosd.toml [env] LIST isn't a plain value (found array); ignoring it`,
				`gosd.toml [env] TABLE isn't a plain value (found table); ignoring it`,
				`gosd.toml [env] WHEN isn't a plain value (found time.Time); ignoring it`,
			},
		},
		{
			name: "a malformed [env] entry still lets hostname parse",
			data: `
hostname = "my-device"

[env]
BAD = [1, 2, 3]
`,
			want:         Config{Hostname: "my-device"},
			wantWarnings: []string{`gosd.toml [env] BAD isn't a plain value (found array); ignoring it`},
		},
		{
			name: "data_flush true enables the flush override",
			data: `data_flush = true`,
			want: Config{DataFlush: boolPtr(true)},
		},
		{
			name: "data_flush false disables the flush override",
			data: `data_flush = false`,
			want: Config{DataFlush: boolPtr(false)},
		},
		{
			name: "missing data_flush leaves the baked default in effect",
			data: `hostname = "my-device"`,
			want: Config{Hostname: "my-device"},
		},
		{
			name: "a quoted data_flush is coerced, with a warning",
			data: `data_flush = "true"`,
			want: Config{DataFlush: boolPtr(true)},
			wantWarnings: []string{
				`gosd.toml data_flush is a quoted "true", not a bare boolean; using true — remove the quotes to silence this warning`,
			},
		},
		{
			name: "a data_flush that isn't true/false falls back to the baked default, with a warning",
			data: `data_flush = "yes"`,
			want: Config{},
			wantWarnings: []string{
				`gosd.toml data_flush "yes" is not true or false; using the baked default`,
			},
		},
		{
			name: "a non-boolean data_flush falls back to the baked default, with a warning",
			data: `data_flush = 1`,
			want: Config{},
			wantWarnings: []string{
				`gosd.toml data_flush isn't a plain boolean (found number); using the baked default`,
			},
		},
		{
			name: "a malformed data_flush still lets hostname parse",
			data: `
hostname = "my-device"
data_flush = [1, 2, 3]
`,
			want: Config{Hostname: "my-device"},
			wantWarnings: []string{
				`gosd.toml data_flush isn't a plain boolean (found array); using the baked default`,
			},
		},
		{
			name: "full ingress config parses",
			data: `
[ingress.cloudflared]
token = "example-tunnel-token"
hostname = "app.example.com"
port = 8080
`,
			want: Config{Ingress: Ingress{Cloudflared: IngressCloudflared{
				Token:    "example-tunnel-token",
				Hostname: "app.example.com",
				Port:     8080,
			}}},
		},
		{
			name: "missing [ingress.cloudflared] table leaves Ingress zero",
			data: `hostname = "my-device"`,
			want: Config{Hostname: "my-device"},
		},
		{
			name: "bare scalars under [ingress.cloudflared] are coerced, with a key-only warning each",
			data: `
[ingress.cloudflared]
token = 123456789
hostname = true
port = "8080"
`,
			want: Config{Ingress: Ingress{Cloudflared: IngressCloudflared{
				Token:    "123456789",
				Hostname: "true",
				Port:     8080,
			}}},
			wantWarnings: []string{
				`gosd.toml [ingress.cloudflared] token is a bare number, not a quoted string; using it as text — add quotes to silence this warning`,
				`gosd.toml [ingress.cloudflared] hostname is a bare boolean, not a quoted string; using it as text — add quotes to silence this warning`,
				`gosd.toml [ingress.cloudflared] port is a quoted number, not a bare integer; using it — remove the quotes to silence this warning`,
			},
		},
		{
			name: "ingress warning order is deterministic (token, hostname, port), regardless of file order",
			data: `
[ingress.cloudflared]
port = "8080"
hostname = true
token = 123456789
`,
			want: Config{Ingress: Ingress{Cloudflared: IngressCloudflared{
				Token:    "123456789",
				Hostname: "true",
				Port:     8080,
			}}},
			wantWarnings: []string{
				`gosd.toml [ingress.cloudflared] token is a bare number, not a quoted string; using it as text — add quotes to silence this warning`,
				`gosd.toml [ingress.cloudflared] hostname is a bare boolean, not a quoted string; using it as text — add quotes to silence this warning`,
				`gosd.toml [ingress.cloudflared] port is a quoted number, not a bare integer; using it — remove the quotes to silence this warning`,
			},
		},
		{
			name: "non-scalar ingress values are dropped, with a warning each",
			data: `
[ingress.cloudflared]
token = ["a", "b"]
hostname = { x = 1 }
port = 2026-07-08T00:00:00Z
`,
			want: Config{},
			wantWarnings: []string{
				`gosd.toml [ingress.cloudflared] token isn't a plain value (found array); ignoring it`,
				`gosd.toml [ingress.cloudflared] hostname isn't a plain value (found table); ignoring it`,
				`gosd.toml [ingress.cloudflared] port isn't a plain value (found time.Time); ignoring it`,
			},
		},
		{
			name: "an ingress port that isn't all digits when quoted is dropped, with a warning",
			data: `
[ingress.cloudflared]
port = "80-80"
`,
			want: Config{},
			wantWarnings: []string{
				`gosd.toml [ingress.cloudflared] port is not a whole number; ignoring it`,
			},
		},
		{
			name: "a malformed ingress entry still lets hostname parse",
			data: `
hostname = "my-device"

[ingress.cloudflared]
token = ["a", "b"]
`,
			want:         Config{Hostname: "my-device"},
			wantWarnings: []string{`gosd.toml [ingress.cloudflared] token isn't a plain value (found array); ignoring it`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, warnings, err := Parse([]byte(tt.data))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Parse(%q) = nil error, want error", tt.data)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", tt.data, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Parse(%q) = %+v, want %+v", tt.data, got, tt.want)
			}
			if !reflect.DeepEqual(warnings, tt.wantWarnings) {
				t.Fatalf("Parse(%q) warnings = %v, want %v", tt.data, warnings, tt.wantWarnings)
			}
		})
	}
}

func boolPtr(b bool) *bool { return &b }

// TestIngressWarningsNeverIncludeTheTokenValue guards the locked decision
// that [ingress.cloudflared] warnings name only the key, never the value:
// the token is a secret and must never end up in a log line, however it's
// malformed. Each case embeds a distinctive marker in the raw token value
// and scans every warning Parse returns (not just the token-specific one -
// a bug could just as easily leak it into an unrelated message) for it.
func TestIngressWarningsNeverIncludeTheTokenValue(t *testing.T) {
	const secretMarker = "sk-super-secret-tunnel-token-should-never-leak"

	tests := []struct {
		name string
		data string
	}{
		{
			name: "token coerced from a bare number",
			data: "[ingress.cloudflared]\ntoken = 8471936502\n",
		},
		{
			name: "token coerced from a bare boolean",
			data: "[ingress.cloudflared]\ntoken = true\n",
		},
		{
			name: "token dropped as an array",
			data: `[ingress.cloudflared]
token = ["` + secretMarker + `"]
`,
		},
		{
			name: "token dropped as an inline table",
			data: `[ingress.cloudflared]
token = { value = "` + secretMarker + `" }
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, warnings, err := Parse([]byte(tt.data))
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", tt.data, err)
			}
			if len(warnings) == 0 {
				t.Fatalf("Parse(%q) produced no warnings, want at least one", tt.data)
			}
			for _, w := range warnings {
				if strings.Contains(w, secretMarker) || strings.Contains(w, "8471936502") {
					t.Errorf("Parse(%q) warning %q leaks the token value", tt.data, w)
				}
			}
		})
	}
}

func TestIngressCloudflaredConfigured(t *testing.T) {
	tests := []struct {
		name string
		c    IngressCloudflared
		want bool
	}{
		{name: "zero value", c: IngressCloudflared{}, want: false},
		{name: "token only", c: IngressCloudflared{Token: "t"}, want: true},
		{name: "hostname only", c: IngressCloudflared{Hostname: "app.example.com"}, want: true},
		{name: "port only", c: IngressCloudflared{Port: 8080}, want: true},
		{name: "all fields", c: IngressCloudflared{Token: "t", Hostname: "app.example.com", Port: 8080}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.c.Configured(); got != tt.want {
				t.Errorf("%+v.Configured() = %v, want %v", tt.c, got, tt.want)
			}
		})
	}
}
