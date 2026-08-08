package main

import (
	"errors"
	"flag"
	"testing"
	"time"
)

func TestParseFlags_Valid(t *testing.T) {
	cfg, err := parseFlags([]string{
		"--statedir", "/data/.gosd/tailscale",
		"--hostname", "my-device",
		"--backend", "http://localhost:8080",
	})
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}
	if cfg.statedir != "/data/.gosd/tailscale" {
		t.Errorf("statedir = %q", cfg.statedir)
	}
	if cfg.hostname != "my-device" {
		t.Errorf("hostname = %q", cfg.hostname)
	}
	if cfg.backend.String() != "http://localhost:8080" {
		t.Errorf("backend = %q", cfg.backend.String())
	}
	if cfg.funnelPort != 443 {
		t.Errorf("funnelPort default = %d, want 443", cfg.funnelPort)
	}
	if cfg.registerTimeout != 5*time.Minute {
		t.Errorf("registerTimeout default = %s, want 5m", cfg.registerTimeout)
	}
}

func TestParseFlags_Overrides(t *testing.T) {
	cfg, err := parseFlags([]string{
		"--statedir", "/data/.gosd/tailscale",
		"--hostname", "my-device",
		"--backend", "http://localhost:9090",
		"--funnel-port", "8443",
		"--register-timeout", "90s",
	})
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}
	if cfg.funnelPort != 8443 {
		t.Errorf("funnelPort = %d, want 8443", cfg.funnelPort)
	}
	if cfg.registerTimeout != 90*time.Second {
		t.Errorf("registerTimeout = %s, want 90s", cfg.registerTimeout)
	}
}

func TestParseFlags_Errors(t *testing.T) {
	base := []string{
		"--statedir", "/data/.gosd/tailscale",
		"--hostname", "my-device",
		"--backend", "http://localhost:8080",
	}
	withOverride := func(flag, value string) []string {
		args := make([]string, 0, len(base))
		skip := false
		for i := 0; i < len(base); i += 2 {
			if base[i] == flag {
				skip = true
				continue
			}
			args = append(args, base[i], base[i+1])
		}
		if !skip {
			args = append(args, flag, value)
		}
		return args
	}

	tests := []struct {
		name string
		args []string
	}{
		{"missing statedir", []string{"--hostname", "my-device", "--backend", "http://localhost:8080"}},
		{"missing hostname", []string{"--statedir", "/data", "--backend", "http://localhost:8080"}},
		{"missing backend", []string{"--statedir", "/data", "--hostname", "my-device"}},
		{"backend has no scheme", withOverride("--backend", "localhost:8080")},
		{"backend has no host", withOverride("--backend", "http://")},
		{"funnel port not in allowed set", withOverride("--funnel-port", "8080")},
		{"register timeout not positive", withOverride("--register-timeout", "0s")},
		{"unexpected positional argument", append(append([]string{}, base...), "extra")},
		{"unknown flag", []string{"--nope", "x"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := parseFlags(tt.args); err == nil {
				t.Fatal("expected an error, got nil")
			}
		})
	}
}

func TestParseFlags_Help(t *testing.T) {
	_, err := parseFlags([]string{"--help"})
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("expected flag.ErrHelp, got %v", err)
	}
}

func TestParseFlags_NeverExposesAuthKeyFlag(t *testing.T) {
	// TS_AUTHKEY must only ever travel as an environment variable (see
	// config's doc comment) — a --authkey/--auth-key flag would appear in
	// gosd-init's supervisor's logged argv, which the epic decision
	// explicitly forbids.
	for _, name := range []string{"--authkey", "--auth-key", "--ts-authkey"} {
		if _, err := parseFlags([]string{name, "tskey-auth-example"}); err == nil {
			t.Fatalf("flag %s unexpectedly accepted", name)
		}
	}
}
