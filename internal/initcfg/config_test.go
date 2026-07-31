package initcfg

import (
	"reflect"
	"testing"
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
