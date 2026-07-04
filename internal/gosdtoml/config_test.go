package gosdtoml

import "testing"

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		want    Config
		wantErr bool
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse([]byte(tt.data))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Parse(%q) = nil error, want error", tt.data)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", tt.data, err)
			}
			if got != tt.want {
				t.Fatalf("Parse(%q) = %+v, want %+v", tt.data, got, tt.want)
			}
		})
	}
}
