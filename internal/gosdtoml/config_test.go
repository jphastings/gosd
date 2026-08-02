package gosdtoml

import (
	"reflect"
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
