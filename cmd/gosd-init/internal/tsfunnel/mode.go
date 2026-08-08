package tsfunnel

import (
	"fmt"

	"github.com/jphastings/gosd/internal/gosdtoml"
)

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

// defaultFunnelPort is used when [ingress.tailscale-funnel] doesn't set
// funnel_port (locked decision: 443 default).
const defaultFunnelPort = 443

// allowedFunnelPorts is the fixed set of ports Tailscale Funnel supports
// (epic gosd-65uy's "Funnel facts": 443, 8443, 10000, TCP only).
var allowedFunnelPorts = map[int]bool{443: true, 8443: true, 10000: true}

// resolveMode decides whether the tailscale-funnel shim should run at all,
// and validates everything about [ingress.tailscale-funnel] that can be
// checked without touching the network, the clock, or the filesystem —
// deliberately pure so mode_test.go can assert on it directly. gosd.toml is
// only ever read once, at boot (nothing here or in Run self-heals a bad
// value later), so every misconfiguration this finds produces exactly one
// actionable log line and a run=false result; the failure modes below are
// the bean's locked list.
//
// Unlike cloudflared's token/hostname/port trio, only port is a required
// key here: the auth key is needed solely for this device's first tailnet
// registration and is safe to remove afterwards (epic decision 4), and
// hostname defaults to deviceHostname (the device's own effective
// hostname) when [ingress.tailscale-funnel] doesn't set one.
func resolveMode(cfg gosdtoml.IngressTailscaleFunnel, baked bool, deviceHostname string) resolvedMode {
	configured := cfg.Configured()

	switch {
	case !configured && !baked:
		// Nothing declared, nothing baked: the overwhelmingly common case
		// for a device with no ingress. Not worth a log line at all.
		return resolvedMode{}
	case !configured && baked:
		return resolvedMode{log: "tailscale-funnel: binary is baked into this image, but [ingress.tailscale-funnel] isn't configured in gosd.toml; nothing to do"}
	case configured && !baked:
		return resolvedMode{log: "tailscale-funnel: [ingress.tailscale-funnel] is configured in gosd.toml, but this image wasn't built with --ingress tailscale-funnel; rebuild with that flag to bake the binary in"}
	}

	if cfg.Port == 0 {
		return resolvedMode{log: "tailscale-funnel: [ingress.tailscale-funnel] is missing required key: port"}
	}
	if cfg.Port < 1 || cfg.Port > 65535 {
		return resolvedMode{log: fmt.Sprintf("tailscale-funnel: [ingress.tailscale-funnel] port %d is out of range (must be 1-65535)", cfg.Port)}
	}

	funnelPort := cfg.FunnelPort
	if funnelPort == 0 {
		funnelPort = defaultFunnelPort
	} else if !allowedFunnelPorts[funnelPort] {
		return resolvedMode{log: fmt.Sprintf("tailscale-funnel: [ingress.tailscale-funnel] funnel_port %d is not one of the supported values (443, 8443, 10000)", funnelPort)}
	}

	hostname := cfg.Hostname
	if hostname == "" {
		hostname = deviceHostname
	}

	return resolvedMode{
		run:        true,
		authkey:    cfg.Authkey,
		hostname:   hostname,
		port:       cfg.Port,
		funnelPort: funnelPort,
	}
}
