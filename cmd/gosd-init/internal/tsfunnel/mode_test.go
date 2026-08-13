package tsfunnel

import (
	"strings"
	"testing"
)

const testAuthkey = "tskey-auth-super-secret-value-do-not-log-me"

func TestResolveModeFailureModes(t *testing.T) {
	tests := []struct {
		name           string
		cfg            Config
		baked          bool
		deviceHostname string
		wantRun        bool
		wantLog        string // substring; "" means "no log line at all"
		notWantLog     string // substring that must never appear (e.g. the authkey)
	}{
		{
			name:    "unconfigured and not baked: silent no-op",
			cfg:     Config{},
			baked:   false,
			wantRun: false,
			wantLog: "",
		},
		{
			name:    "unconfigured but baked: one quiet line",
			cfg:     Config{},
			baked:   true,
			wantRun: false,
			wantLog: "nothing to do",
		},
		{
			name:    "configured but not baked: points at --ingress",
			cfg:     Config{Authkey: testAuthkey, Port: "8080"},
			baked:   false,
			wantRun: false,
			wantLog: "--ingress tailscale-funnel",
		},
		{
			name:       "missing port",
			cfg:        Config{Authkey: testAuthkey, Hostname: "device-name"},
			baked:      true,
			wantRun:    false,
			wantLog:    "missing required setting: port",
			notWantLog: testAuthkey,
		},
		{
			name:    "port out of range low",
			cfg:     Config{Port: "-1"},
			baked:   true,
			wantRun: false,
			wantLog: "out of range",
		},
		{
			name:    "port out of range high",
			cfg:     Config{Port: "70000"},
			baked:   true,
			wantRun: false,
			wantLog: "out of range",
		},
		{
			name:       "funnel_port outside the allowed set",
			cfg:        Config{Authkey: testAuthkey, Port: "8080", FunnelPort: "9999"},
			baked:      true,
			wantRun:    false,
			wantLog:    "not one of the supported values (443, 8443, 10000)",
			notWantLog: testAuthkey,
		},
		{
			name:    "port only: no authkey needed once state exists",
			cfg:     Config{Port: "8080"},
			baked:   true,
			wantRun: true,
			wantLog: "",
		},
		{
			name:    "fully valid: runs, no log line",
			cfg:     Config{Authkey: testAuthkey, Hostname: "my-device", Port: "8080", FunnelPort: "8443"},
			baked:   true,
			wantRun: true,
			wantLog: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := resolveMode(tt.cfg, tt.baked, tt.deviceHostname)
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
				t.Errorf("log = %q leaked the authkey %q", m.log, tt.notWantLog)
			}
		})
	}
}

func TestResolveModeDefaultsFunnelPortTo443(t *testing.T) {
	m := resolveMode(Config{Port: "8080"}, true, "device-name")
	if !m.run {
		t.Fatalf("run = false, want true (log: %q)", m.log)
	}
	if m.funnelPort != 443 {
		t.Errorf("funnelPort = %d, want 443", m.funnelPort)
	}
}

func TestResolveModeHostnameDefaultsToDeviceHostname(t *testing.T) {
	m := resolveMode(Config{Port: "8080"}, true, "my-device")
	if !m.run {
		t.Fatalf("run = false, want true (log: %q)", m.log)
	}
	if m.hostname != "my-device" {
		t.Errorf("hostname = %q, want the device hostname %q", m.hostname, "my-device")
	}
}

func TestResolveModeExplicitHostnameOverridesDeviceHostname(t *testing.T) {
	m := resolveMode(Config{Hostname: "custom", Port: "8080"}, true, "my-device")
	if !m.run {
		t.Fatalf("run = false, want true (log: %q)", m.log)
	}
	if m.hostname != "custom" {
		t.Errorf("hostname = %q, want the explicit config value %q", m.hostname, "custom")
	}
}

func TestResolveModeValidConfigPopulatesFields(t *testing.T) {
	cfg := Config{Authkey: testAuthkey, Hostname: "my-device", Port: "8080", FunnelPort: "8443"}
	m := resolveMode(cfg, true, "fallback-hostname")

	if !m.run {
		t.Fatalf("run = false, want true (log: %q)", m.log)
	}
	if m.authkey != testAuthkey {
		t.Errorf("authkey = %q, want %q", m.authkey, testAuthkey)
	}
	if m.hostname != "my-device" || m.port != 8080 || m.funnelPort != 8443 {
		t.Errorf("hostname/port/funnelPort = %s/%d/%d, want my-device/8080/8443", m.hostname, m.port, m.funnelPort)
	}
}
