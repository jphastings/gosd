package initcfg

import (
	"reflect"
	"testing"
	"time"
)

func TestParseConfig(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		want    Config
		wantErr bool
	}{
		{
			name: "full config",
			data: `{"board":"pi-zero-2w","hostname":"my-device","wifi":{"ssid":"home","passphrase":"hunter2"}}`,
			want: Config{
				Board:    "pi-zero-2w",
				Hostname: "my-device",
				Wifi:     Wifi{SSID: "home", Passphrase: "hunter2"},
			},
		},
		{
			name: "partial config leaves the rest zero",
			data: `{"hostname":"my-device"}`,
			want: Config{Hostname: "my-device"},
		},
		{
			name: "config predating ntpServers parses unchanged",
			data: `{"board":"pi-zero-2w","hostname":"my-device","wifi":{"ssid":"home","passphrase":"hunter2"}}`,
			want: Config{
				Board:    "pi-zero-2w",
				Hostname: "my-device",
				Wifi:     Wifi{SSID: "home", Passphrase: "hunter2"},
			},
		},
		{
			name: "dataExpand marks an expand-on-first-boot image",
			data: `{"hostname":"my-device","dataExpand":true}`,
			want: Config{Hostname: "my-device", DataExpand: true},
		},
		{
			name: "dataFlush marks a build with --data-flush baked in",
			data: `{"hostname":"my-device","dataFlush":true}`,
			want: Config{Hostname: "my-device", DataFlush: true},
		},
		{
			name: "config predating dataFlush parses unchanged, not as an error",
			data: `{"hostname":"my-device"}`,
			want: Config{Hostname: "my-device"},
		},
		{
			name: "ingressCloudflared marks a build with --ingress cloudflared baked in",
			data: `{"hostname":"my-device","ingressCloudflared":true}`,
			want: Config{Hostname: "my-device", IngressCloudflared: true},
		},
		{
			name: "config predating ingressCloudflared parses unchanged, not as an error",
			data: `{"hostname":"my-device"}`,
			want: Config{Hostname: "my-device"},
		},
		{
			name: "ingressTailscaleFunnel marks a build with --ingress tailscale-funnel baked in",
			data: `{"hostname":"my-device","ingressTailscaleFunnel":true}`,
			want: Config{Hostname: "my-device", IngressTailscaleFunnel: true},
		},
		{
			name: "config predating ingressTailscaleFunnel parses unchanged, not as an error",
			data: `{"hostname":"my-device"}`,
			want: Config{Hostname: "my-device"},
		},
		{
			name: "identity parses when present",
			data: `{"hostname":"my-device","identity":"deadbeef"}`,
			want: Config{Hostname: "my-device", Identity: "deadbeef"},
		},
		{
			name: "config predating identity parses unchanged, not as an error",
			data: `{"hostname":"my-device"}`,
			want: Config{Hostname: "my-device"},
		},
		{
			name: "ntpServers overrides the default list",
			data: `{"hostname":"my-device","ntpServers":["ntp1.example.com","ntp2.example.com"]}`,
			want: Config{
				Hostname:   "my-device",
				NTPServers: []string{"ntp1.example.com", "ntp2.example.com"},
			},
		},
		{
			name: "missing file (empty data) is not an error",
			data: "",
			want: Config{},
		},
		{
			name:    "garbage input is reported, not panicked",
			data:    "{not json",
			wantErr: true,
		},
		{
			name: "buildTimestamp parses when present",
			data: `{"hostname":"my-device","buildTimestamp":"2026-07-31T12:00:00Z"}`,
			want: Config{Hostname: "my-device", BuildTimestamp: "2026-07-31T12:00:00Z"},
		},
		{
			name: "config predating buildTimestamp parses unchanged, not as an error",
			data: `{"hostname":"my-device"}`,
			want: Config{Hostname: "my-device"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseConfig([]byte(tt.data))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseConfig(%q) = nil error, want error", tt.data)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseConfig(%q) unexpected error: %v", tt.data, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ParseConfig(%q) = %+v, want %+v", tt.data, got, tt.want)
			}
		})
	}
}

func TestConfigShortIdentity(t *testing.T) {
	tests := []struct {
		name     string
		identity string
		want     string
	}{
		{name: "empty identity (pre-gosd-acdn image) stays empty", identity: "", want: ""},
		{name: "short identity is returned whole", identity: "abcd", want: "abcd"},
		{
			name:     "a full sha256 hex digest is truncated",
			identity: "30e629b6f8caf1ff8f16ee98d8f1c5c7eb3138b9c63944e235e9678744f2094b",
			want:     "30e629b6f8ca",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{Identity: tt.identity}
			if got := cfg.ShortIdentity(); got != tt.want {
				t.Errorf("Config{Identity: %q}.ShortIdentity() = %q, want %q", tt.identity, got, tt.want)
			}
		})
	}
}

func TestConfigBuildTime(t *testing.T) {
	tests := []struct {
		name      string
		timestamp string
		want      time.Time
	}{
		{name: "empty timestamp (pre-gosd-0esw image) yields the zero time, not the epoch", timestamp: "", want: time.Time{}},
		{
			name:      "RFC3339Nano round-trips",
			timestamp: "2026-07-31T12:00:00.123456789Z",
			want:      time.Date(2026, 7, 31, 12, 0, 0, 123456789, time.UTC),
		},
		{
			name:      "RFC3339 without a fractional part also parses",
			timestamp: "2026-07-31T12:00:00Z",
			want:      time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC),
		},
		{name: "malformed timestamp yields the zero time rather than an error", timestamp: "not-a-time", want: time.Time{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{BuildTimestamp: tt.timestamp}
			if got := cfg.BuildTime(); !got.Equal(tt.want) {
				t.Errorf("Config{BuildTimestamp: %q}.BuildTime() = %v, want %v", tt.timestamp, got, tt.want)
			}
		})
	}
}
