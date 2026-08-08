package main

import (
	"flag"
	"fmt"
	"net/url"
	"time"
)

// config is the fully parsed, validated result of parseFlags — everything
// run needs to start tsnet and the reverse proxy. TS_AUTHKEY deliberately
// has no flag: gosd-init's supervisor logs the argv of every child it
// starts, but never its environment, so a secret that must never appear in
// a log line has to travel as an env var. tsnet.Server reads TS_AUTHKEY
// straight from the environment itself when its AuthKey field is left
// empty (see main's Server literal) — there is nothing for gosd-tsfunnel to
// wire by hand.
type config struct {
	statedir        string
	hostname        string
	backend         *url.URL
	funnelPort      int
	registerTimeout time.Duration
}

// parseFlags parses and validates args, returning an actionable error for
// anything wrong that doesn't need the network, the tailnet, or the
// filesystem to detect.
func parseFlags(args []string) (config, error) {
	fs := flag.NewFlagSet("gosd-tsfunnel", flag.ContinueOnError)
	statedir := fs.String("statedir", "", "directory to store tsnet state in (required; must persist across reboots — losing it creates a new node identity and a new public URL)")
	hostname := fs.String("hostname", "", "hostname to present to the tailnet (required)")
	backend := fs.String("backend", "", "local app URL to proxy Funnel traffic to, e.g. http://localhost:8080 (required)")
	funnelPort := fs.Int("funnel-port", 443, "Funnel port to listen on (443, 8443, or 10000)")
	registerTimeout := fs.Duration("register-timeout", 5*time.Minute, "how long to wait for tsnet to finish registering with the tailnet before giving up")

	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	if extra := fs.Args(); len(extra) > 0 {
		return config{}, fmt.Errorf("unexpected argument(s): %v", extra)
	}

	if *statedir == "" {
		return config{}, fmt.Errorf("--statedir is required")
	}
	if *hostname == "" {
		return config{}, fmt.Errorf("--hostname is required")
	}
	if *backend == "" {
		return config{}, fmt.Errorf("--backend is required")
	}
	backendURL, err := url.Parse(*backend)
	if err != nil || backendURL.Scheme == "" || backendURL.Host == "" {
		return config{}, fmt.Errorf("--backend %q must be an absolute URL, e.g. http://localhost:8080", *backend)
	}
	switch *funnelPort {
	case 443, 8443, 10000:
	default:
		return config{}, fmt.Errorf("--funnel-port %d is not supported by Tailscale Funnel (must be 443, 8443, or 10000)", *funnelPort)
	}
	if *registerTimeout <= 0 {
		return config{}, fmt.Errorf("--register-timeout must be positive, got %s", *registerTimeout)
	}

	return config{
		statedir:        *statedir,
		hostname:        *hostname,
		backend:         backendURL,
		funnelPort:      *funnelPort,
		registerTimeout: *registerTimeout,
	}, nil
}
