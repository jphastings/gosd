// Command gosd-tsfunnel exposes a local app on the public internet via
// Tailscale Funnel, using tsnet's pure-Go userspace netstack: no
// /dev/net/tun, no root, no iptables (epic gosd-65uy, decision 1). It
// registers a node on the operator's tailnet (TS_AUTHKEY, read from the
// environment only — see config's doc comment for why), waits for that
// registration to finish, opens a Funnel listener, and reverse-proxies
// every request to a local backend.
//
// gosd-tsfunnel never retries internally: a fatal error here means the
// caller (gosd-init's supervisor) should back off and restart the whole
// process. Both of the failure modes below are usually fixed out-of-band —
// a tailnet ACL edit, a fresh auth key — so a restart genuinely gives the
// next attempt a chance to succeed once that's done; an in-process retry
// loop would just duplicate the supervisor's own backoff.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"

	"tailscale.com/tsnet"
)

func main() {
	if err := run(os.Args[1:], os.Stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		fmt.Fprintln(os.Stderr, "gosd-tsfunnel: "+err.Error())
		os.Exit(1)
	}
}

func run(args []string, stderr io.Writer) error {
	cfg, err := parseFlags(args)
	if err != nil {
		return err
	}

	srv := &tsnet.Server{
		Dir:      cfg.statedir,
		Hostname: cfg.hostname,
		// Logf is the backend's own verbose debug log (magicsock, netstack,
		// the LocalBackend state machine, ...) — it would flood a 115200
		// serial console, so it's discarded outright. UserLogf carries the
		// messages meant for a human (the interactive login URL, status
		// changes): those go to stderr, which the supervisor prefixes and
		// forwards to the console (epic gosd-65uy, decision on logging).
		Logf: func(string, ...any) {},
		UserLogf: func(format string, args ...any) {
			_, _ = fmt.Fprintf(stderr, format+"\n", args...)
		},
	}
	defer func() { _ = srv.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), cfg.registerTimeout)
	defer cancel()
	if _, err := srv.Up(ctx); err != nil {
		return registerTimeoutError(cfg.registerTimeout, err)
	}

	ln, err := srv.ListenFunnel("tcp", fmt.Sprintf(":%d", cfg.funnelPort))
	if err != nil {
		return funnelUnavailableError(err)
	}
	defer func() { _ = ln.Close() }()

	return http.Serve(ln, newReverseProxy(cfg.backend))
}
