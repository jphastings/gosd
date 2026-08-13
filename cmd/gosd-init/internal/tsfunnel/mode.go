package tsfunnel

import (
	"fmt"
	"strconv"

	"github.com/jphastings/gosd/internal/configtree"
)

// settingsDir is where the card declares its Tailscale Funnel: one file per
// setting, named in every line this package logs so a person reading the
// serial console knows which file to go and open.
const settingsDir = configtree.Dir + "/ingress/tailscale-funnel"

// Config is the Tailscale Funnel a card declares, read straight off it: a
// tagged, reusable auth key from the tailnet's admin console (tagging a key
// disables that node's key expiry, so a shipped device never gets locked
// out — see epic gosd-65uy decision 4), the local app port to expose, and
// two optional settings — the public hostname (defaults to the device's own
// hostname) and which of Tailscale's three funnel ports to use (defaults to
// 443). The auth key is needed only for this device's first registration:
// tsnet ignores it once local state already exists, so it's safe to clear
// from the card afterwards.
//
// Every field is text because every setting is text — a file somebody typed
// into (see cmd/gosd-init/internal/cardconfig) — and resolveMode is the
// single place that decides whether what they typed can be used, exactly as
// cloudflared.Config's is.
type Config struct {
	Authkey    string
	Hostname   string
	Port       string
	FunnelPort string
}

// Configured reports whether any setting has been given a value, i.e.
// whether this card declares (or attempts to declare) a Tailscale Funnel at
// all.
func (c Config) Configured() bool {
	return c.Authkey != "" || c.Hostname != "" || c.Port != "" || c.FunnelPort != ""
}

// resolvedMode is the pure result of resolveMode: either run is true and
// the remaining fields hold everything runArgs/runEnv need to build the
// shim's argv/env, or run is false and log (if non-empty) is the single
// actionable line Run should log before returning. A false run with an
// empty log is the ordinary "nothing configured, nothing baked" case, which
// needs no log line at all.
type resolvedMode struct {
	run bool
	log string

	authkey, hostname string
	port, funnelPort  int
}

// defaultFunnelPort is used when the card doesn't set funnel_port (locked
// decision: 443 default).
const defaultFunnelPort = 443

// allowedFunnelPorts is the fixed set of ports Tailscale Funnel supports
// (epic gosd-65uy's "Funnel facts": 443, 8443, 10000, TCP only).
var allowedFunnelPorts = map[int]bool{443: true, 8443: true, 10000: true}

// resolveMode decides whether the tailscale-funnel shim should run at all,
// and validates everything about the card's tailscale-funnel settings that
// can be checked without touching the network, the clock, or the filesystem
// — deliberately pure so mode_test.go can assert on it directly. The card
// is only ever read once, at boot (nothing here or in Run self-heals a bad
// value later), so every misconfiguration this finds produces exactly one
// actionable log line and a run=false result; the failure modes below are
// the bean's locked list.
//
// Unlike cloudflared's token/hostname/port trio, only port is required
// here: the auth key is needed solely for this device's first tailnet
// registration and is safe to remove afterwards (epic decision 4), and
// hostname defaults to deviceHostname (the device's own effective hostname)
// when the card doesn't name one.
func resolveMode(cfg Config, baked bool, deviceHostname string) resolvedMode {
	configured := cfg.Configured()

	switch {
	case !configured && !baked:
		// Nothing declared, nothing baked: the overwhelmingly common case
		// for a device with no ingress. Not worth a log line at all.
		return resolvedMode{}
	case !configured && baked:
		return resolvedMode{log: fmt.Sprintf("tailscale-funnel: binary is baked into this image, but nothing is set in %s; nothing to do", settingsDir)}
	case configured && !baked:
		return resolvedMode{log: fmt.Sprintf("tailscale-funnel: %s is set on the card, but this image wasn't built with --ingress tailscale-funnel; rebuild with that flag to bake the binary in", settingsDir)}
	}

	if cfg.Port == "" {
		return resolvedMode{log: fmt.Sprintf("tailscale-funnel: %s is missing required setting: port", settingsDir)}
	}
	port, err := strconv.Atoi(cfg.Port)
	if err != nil {
		return resolvedMode{log: fmt.Sprintf("tailscale-funnel: %s/port %q is not a whole number", settingsDir, cfg.Port)}
	}
	if port < 1 || port > 65535 {
		return resolvedMode{log: fmt.Sprintf("tailscale-funnel: %s/port %d is out of range (must be 1-65535)", settingsDir, port)}
	}

	funnelPort := defaultFunnelPort
	if cfg.FunnelPort != "" {
		funnelPort, err = strconv.Atoi(cfg.FunnelPort)
		if err != nil {
			return resolvedMode{log: fmt.Sprintf("tailscale-funnel: %s/funnel_port %q is not a whole number", settingsDir, cfg.FunnelPort)}
		}
		if !allowedFunnelPorts[funnelPort] {
			return resolvedMode{log: fmt.Sprintf("tailscale-funnel: %s/funnel_port %d is not one of the supported values (443, 8443, 10000)", settingsDir, funnelPort)}
		}
	}

	hostname := cfg.Hostname
	if hostname == "" {
		hostname = deviceHostname
	}

	return resolvedMode{
		run:        true,
		authkey:    cfg.Authkey,
		hostname:   hostname,
		port:       port,
		funnelPort: funnelPort,
	}
}
