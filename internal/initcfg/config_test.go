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
