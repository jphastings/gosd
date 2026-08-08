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
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

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

	if err := prepareStateDir(cfg.statedir); err != nil {
		return fmt.Errorf("preparing tsnet state directory %q: %w", cfg.statedir, err)
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

	ctx, cancel := context.WithTimeout(context.Background(), cfg.registerTimeout)
	defer cancel()
	if _, err := srv.Up(ctx); err != nil {
		// srv.Close() is deliberately NOT deferred until after a successful
		// Up (see the defer below): tsnet's Close panics ("unreachable"
		// from tsdial.PeerAPIHTTPClient) when Up failed before the dialer
		// finished initializing, which would crash this process and mask
		// the real Up error being returned here. The process exits right
		// after this return, so the OS reclaims what Close would have
		// released (bean gosd-6cf2).
		return registerTimeoutError(cfg.registerTimeout, err)
	}
	defer func() { _ = srv.Close() }()

	ln, err := srv.ListenFunnel("tcp", fmt.Sprintf(":%d", cfg.funnelPort))
	if err != nil {
		return funnelUnavailableError(err)
	}
	defer func() { _ = ln.Close() }()

	return http.Serve(ln, newReverseProxy(cfg.backend))
}

// tsnetStateFiles are the JSON files tsnet.Server persists directly under
// its state directory across restarts — the node's private key and tailnet
// identity (tailscaled.state) and its log-upload config (tailscaled.log.conf).
// tsnet writes them as plain JSON with no write-then-rename (bean gosd-e721
// tracks the upstream fix request), so a power cut mid-write can leave one
// empty or truncated.
var tsnetStateFiles = []string{"tailscaled.state", "tailscaled.log.conf"}

// prepareStateDir makes statedir ready for a tsnet.Server rooted there,
// before the Server is ever constructed. It addresses two hardware-only
// startup bugs found on the bench (bean gosd-6cf2), neither reachable by
// tests that don't exec real tsnet in the real gosd-init environment (no
// HOME, initramfs rootfs, /data state surviving reflash):
//
//   - logs-dir panic: tsnet's logpolicy.LogsDir probes, in order,
//     $TS_LOGS_DIR, $STATE_DIRECTORY, /var/lib/tailscale (via MkdirAll),
//     then os.UserCacheDir() — and PANICS if none work. On a GoSD image
//     none do: the initramfs rootfs has no /var/lib/tailscale, and the
//     supervised child has no HOME so UserCacheDir fails too. TS_LOGS_DIR
//     is the first path LogsDir honours, but only if it already exists
//     (LogsDir os.Stats it and ignores it otherwise) — so statedir must be
//     created before TS_LOGS_DIR is set to it.
//
//   - corrupt-state wedge: an empty or truncated tailscaled.state or
//     tailscaled.log.conf makes tsnet.Up refuse to load it ("unexpected
//     end of JSON input") and it never regenerates on its own — a
//     permanent wedge, made sticky by /data surviving a plain Imager
//     reflash. Both files regenerate when absent, the log ID is unshipped
//     telemetry, and an unparseable state file holds no identity worth
//     keeping (fresh registration via TS_AUTHKEY is the correct recovery),
//     so an invalid file is removed here, never repaired. A VALID file is
//     left untouched — node identity must survive reboot and reflash.
func prepareStateDir(statedir string) error {
	if err := os.MkdirAll(statedir, 0o700); err != nil {
		return fmt.Errorf("creating %q: %w", statedir, err)
	}

	for _, name := range tsnetStateFiles {
		path := filepath.Join(statedir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue // absent, or unreadable for a reason Up will report on its own
		}
		if !json.Valid(data) {
			if err := os.Remove(path); err != nil {
				return fmt.Errorf("removing corrupt state file %q: %w", path, err)
			}
		}
	}

	if err := os.Setenv("TS_LOGS_DIR", statedir); err != nil {
		return fmt.Errorf("setting TS_LOGS_DIR: %w", err)
	}
	return nil
}
