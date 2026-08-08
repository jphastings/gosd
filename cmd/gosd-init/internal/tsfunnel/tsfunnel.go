// Package tsfunnel implements gosd-init's supervision of a baked-in
// gosd-tsfunnel shim binary (cmd/gosd-tsfunnel, see epic gosd-65uy),
// exposing a device's [ingress.tailscale-funnel]-declared HTTP service to
// the public internet over Tailscale Funnel with zero app code. The shim
// itself is a tiny tsnet-based process: it holds the tailnet node identity,
// configures Funnel, and reverse-proxies to the local port this package's
// caller resolved from gosd.toml. This package's own job stops at deciding
// whether the shim should run at all, gating its start on network/time
// readiness, preflighting its state directory, and keeping it running.
//
// The node's whole tailnet identity — private key, tailnet membership, the
// public *.ts.net hostname it was assigned — lives at StateDir, on the data
// partition (epic decision 3), not in gosd.toml and not in this package.
// Losing that directory means a new node identity and a new public URL, so
// StateDir must exist and be writable before the shim ever starts; Run
// preflights it explicitly (see the state-dir preflight step below) so a
// read-only /data produces one actionable log line instead of the shim
// silently failing to establish tsnet state.
//
// Following the style established by boot, netup, wifiup, timesync,
// mdnsresponder, and cloudflared, every side-effecting dependency (starting
// the process, waiting for it to exit, the gate marker files, the
// filesystem, the clock, logging) sits behind Deps, so mode resolution,
// gating, and supervision are fully unit-tested with fakes on any OS.
// platform.go's StartProcess is a plain os/exec wrapper needing no "linux"
// build tag (see its doc comment) — like cloudflared, this package has no
// platform_linux.go/platform_other.go split. The line-splitting log writer
// and the restart backoff are the SHARED cmd/gosd-init/internal/logwriter
// and cmd/gosd-init/internal/childbackoff packages (bean gosd-wxjy) —
// imported here, not forked, so this module and cloudflared keep exactly
// the same relay/backoff behavior without duplicating it a second time.
//
// This module ships UNWIRED: nothing in cmd/gosd-init/main.go calls Run
// yet. Wiring Run into the boot sequence — including assigning
// Deps.StartProcess to this package's own StartProcess and Deps.Wait to the
// PID-1 reaper's Wait (see Deps.Wait's doc comment) — is a later bean in
// the same epic (gosd-o68e), once this package has been reviewed in
// isolation.
package tsfunnel

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jphastings/gosd/cmd/gosd-init/internal/childbackoff"
	"github.com/jphastings/gosd/cmd/gosd-init/internal/logwriter"
	"github.com/jphastings/gosd/internal/gosdtoml"
)

// logPrefix is prepended to every line this package logs, including the
// shim's relayed stdout/stderr (see runOnce) — the gosd-o68e wiring bean's
// locked choice, matching the "tailscale-funnel" name used everywhere else
// this feature is user-facing (the --ingress flag value, the gosd.toml
// section name), rather than the shorter "tsfunnel" package/binary name.
const logPrefix = "tailscale-funnel: "

// StateDir is where the shim's tsnet state — including the node's private
// key and tailnet identity — lives on the data partition (epic gosd-65uy
// decision 3). It is created (mode 0700, since it will hold private key
// material) by Run's state-dir preflight before the shim is ever started;
// the shim itself is passed the same path via --statedir.
const StateDir = "/data/.gosd/tailscale"

// Default tuning values for production wiring; tests use their own, much
// shorter values so they don't take real wall-clock time to run.
const (
	// DefaultNetworkUpPollInterval is how often Run polls for the
	// network-up marker while waiting for it to appear, mirroring
	// cloudflared.DefaultNetworkUpPollInterval.
	DefaultNetworkUpPollInterval = 2 * time.Second

	// DefaultTimeSyncedTimeout is how long Run waits for the time-synced
	// marker before giving up and starting the shim anyway (locked
	// decision): tsnet's own registration/ACME retries, plus this
	// package's restart backoff, absorb a clock that's still catching up.
	DefaultTimeSyncedTimeout = 2 * time.Minute

	// DefaultTimeSyncedPollInterval is how often Run polls for the
	// time-synced marker while waiting for it, or for
	// DefaultTimeSyncedTimeout to elapse.
	DefaultTimeSyncedPollInterval = 2 * time.Second
)

// Deps bundles every dependency Run needs. Production wiring (the later
// wiring bean's main.go) supplies real implementations; tests supply fakes.
type Deps struct {
	// StartProcess launches path with args and env, directing its
	// stdout/stderr, and returns its pid without waiting for it to exit.
	// Production wires this to this package's own StartProcess
	// (platform.go). This is a package-local seam rather than a shared
	// boot.AppStarter because boot.AppStarter.Start takes no argv at all
	// (it only ever launches /app) — the shim needs one.
	StartProcess func(path string, args, env []string, stdout, stderr io.Writer) (pid int, err error)

	// Wait blocks until pid has exited, returning its exit status.
	// Production wires this to the PID-1 reaper's Wait
	// (boot.Platform.Reaper.Wait) — NEVER exec.Cmd.Wait. As PID 1,
	// gosd-init reaps every child (including this one) through a single
	// central wait4(-1, ...) loop (boot's linuxReaper); calling
	// exec.Cmd.Wait directly on the *exec.Cmd StartProcess creates
	// internally would race that loop for the same child's exit status —
	// exactly the hazard boot.AppStarter's doc comment, and
	// cloudflared.Deps.Wait's, already document for their own children.
	Wait func(pid int) (status int, err error)

	// NetworkUp reports whether the network-up marker file
	// (netup.DefaultNetworkUpPath in production) currently exists. Run
	// polls this (via waitForNetworkUp) before the state-dir preflight and
	// again before every restart attempt in the supervise loop — paths are
	// injected rather than importing netup, following
	// timesync/cloudflared's precedent of no cross-package imports between
	// gosd-init's side-effecting feature modules.
	NetworkUp func() (bool, error)

	// TimeSynced reports whether the time-synced marker file
	// (timesync.DefaultTimeSyncedPath in production) currently exists. Run
	// waits for it (via waitForTimeSynced) up to Options.TimeSyncedTimeout
	// before proceeding regardless, since Tailscale's control-plane TLS
	// wants a roughly-correct clock but must not block ingress forever if
	// NTP is unreachable.
	TimeSynced func() (bool, error)

	// MkdirAll creates StateDir before the shim is ever started (the
	// state-dir preflight). Matching os.MkdirAll's exact signature lets
	// production wiring assign it directly. A failure here — most notably
	// EROFS, when /data fell back to a read-only mount — is the epic's
	// locked "read-only /data at runtime" case: Run logs one actionable
	// line and returns rather than starting a shim that can never persist
	// its tsnet state.
	MkdirAll func(path string, perm os.FileMode) error

	Clock Clock

	// NewBackoff creates the restart backoff used by the supervise loop. A
	// func rather than a shared *childbackoff.Backoff so that if Run is
	// ever invoked more than once its restart sequence always starts
	// fresh, mirroring cloudflared.Deps.NewBackoff's rationale.
	NewBackoff func() *childbackoff.Backoff

	// Log records what tailscale-funnel supervision is doing. Every line
	// this package logs is prefixed "tailscale-funnel: " (see logPrefix).
	// Never nil in production (wired to boot's console logger). Env is
	// never logged — TS_AUTHKEY lives there (see runEnv) — even in the
	// pid+argv start line, mirroring cloudflared's token-never-logged
	// discipline for its own runtime secret.
	Log func(format string, args ...any)
}

// Options holds the per-boot behavior knobs for tailscale-funnel
// supervision.
type Options struct {
	// BinaryPath is where gosd build baked the gosd-tsfunnel shim for this
	// board. Unused unless Config.Configured() and Baked are both true.
	// Caller-supplied rather than computed here, mirroring
	// cloudflared.Options.BinaryPath: the wiring bean passes in the build
	// rail's own path for the baked binary.
	BinaryPath string

	// Baked reports whether gosd build --ingress tailscale-funnel baked
	// the shim into this image at BinaryPath. Wired from a future
	// config.json bit by the later wiring bean; resolveMode never probes
	// the filesystem for the binary itself (locked decision: the "is it
	// baked" bit lives in config.json, not on disk).
	Baked bool

	// Config is the already-parsed [ingress.tailscale-funnel] section of
	// gosd.toml.
	Config gosdtoml.IngressTailscaleFunnel

	// Hostname is the device's own effective hostname (gosdtoml.Config's
	// top-level Hostname, as resolved elsewhere at boot — the same value
	// mdnsresponder.Options.Hostname is wired from). resolveMode uses it
	// as the default public hostname when Config.Hostname is empty (the
	// locked "hostname defaults to the device hostname" decision).
	Hostname string

	NetworkUpPollInterval  time.Duration
	TimeSyncedTimeout      time.Duration
	TimeSyncedPollInterval time.Duration

	// Stop, if non-nil, ends supervision when closed. Production leaves
	// this nil so Run runs for the life of the process, as PID 1 requires;
	// tests set it to bound the otherwise-infinite loops.
	Stop <-chan struct{}
}

// Run resolves whether the tailscale-funnel shim should run at all (see
// resolveMode), logging the one actionable line and returning immediately
// if not, then waits for the network-up marker (parking forever if it never
// appears), waits up to Options.TimeSyncedTimeout for the time-synced
// marker (proceeding with a warning if it times out), preflights StateDir,
// and supervises the shim for as long as Options.Stop isn't closed.
//
// Run is meant to be launched in its own goroutine (see
// boot.Deps.StartNetworking's sibling calls): per the "no interactive
// surface" / never-block-/app decision, ingress bring-up must never delay
// anything else gosd-init does.
func Run(deps Deps, opts Options) {
	m := resolveMode(opts.Config, opts.Baked, opts.Hostname)
	if !m.run {
		if m.log != "" {
			deps.Log(m.log)
		}
		return
	}

	if !waitForNetworkUp(deps, opts) {
		return
	}
	waitForTimeSynced(deps, opts)

	if err := deps.MkdirAll(StateDir, 0o700); err != nil {
		deps.Log("tailscale-funnel: %s is not writable (%v); tailscale-funnel needs a data partition; rebuild with --data-size", StateDir, err)
		return
	}

	supervise(deps, opts, m)
}

// waitForNetworkUp polls deps.NetworkUp (checking immediately, then every
// opts.NetworkUpPollInterval) until it reports true or opts.Stop closes. It
// returns false only in the latter case. Mirrors
// cloudflared.waitForNetworkUp exactly.
func waitForNetworkUp(deps Deps, opts Options) bool {
	for {
		up, err := deps.NetworkUp()
		if err != nil {
			deps.Log("tailscale-funnel: checking network-up marker failed: %v", err)
		} else if up {
			return true
		}

		select {
		case <-opts.Stop:
			return false
		case <-deps.Clock.After(opts.NetworkUpPollInterval):
		}
	}
}

// waitForTimeSynced polls deps.TimeSynced until it reports true or
// opts.TimeSyncedTimeout elapses, in which case it logs a warning and
// returns anyway — unlike waitForNetworkUp, ingress must not wait forever
// on NTP. It also returns early, with no warning, if opts.Stop closes.
func waitForTimeSynced(deps Deps, opts Options) {
	deadline := deps.Clock.Now().Add(opts.TimeSyncedTimeout)
	for {
		synced, err := deps.TimeSynced()
		if err != nil {
			deps.Log("tailscale-funnel: checking time-synced marker failed: %v", err)
		} else if synced {
			return
		}

		if !deps.Clock.Now().Before(deadline) {
			deps.Log("tailscale-funnel: time sync did not complete within %s; starting the shim anyway", opts.TimeSyncedTimeout)
			return
		}

		select {
		case <-opts.Stop:
			return
		case <-deps.Clock.After(opts.TimeSyncedPollInterval):
		}
	}
}

// runArgs builds the shim's argv from a resolved mode: --statedir,
// --hostname, --backend (the local app port as an http://localhost URL),
// and --funnel-port, per TS-2 (gosd-4fve)'s flag contract. There is
// deliberately no --authkey flag: the auth key travels only through
// TS_AUTHKEY in the child's environment (see runEnv), never in argv, since
// argv is what the supervisor logs on every start (see runOnce).
func runArgs(m resolvedMode) []string {
	return []string{
		"--statedir", StateDir,
		"--hostname", m.hostname,
		"--backend", fmt.Sprintf("http://localhost:%d", m.port),
		"--funnel-port", strconv.Itoa(m.funnelPort),
	}
}

// runEnv builds the shim's environment: TS_AUTHKEY only. An empty auth key
// is fine once tsnet state already exists at StateDir — tsnet ignores it in
// that case (epic decision 4) — so runEnv never special-cases an empty
// m.authkey.
func runEnv(m resolvedMode) []string {
	return []string{"TS_AUTHKEY=" + m.authkey}
}

// supervise starts the shim and restarts it with backoff whenever it exits,
// for as long as opts.Stop isn't closed. Every dependency is injected so
// this is unit-testable without real processes, clocks, or sleeps — the
// same shape as cloudflared.supervise, with the same reasoning for
// re-checking the network-up marker before every single start attempt, not
// just the first: if the shim exits while the network is down, restarting
// it into a network that's still down would only burn backoff for no
// reason, so this parks on the network-up gate instead (locked decision:
// "NO restart on network change").
func supervise(deps Deps, opts Options, m resolvedMode) {
	backoff := deps.NewBackoff()

	for {
		if !waitForNetworkUp(deps, opts) {
			return
		}

		runOnce(deps, opts, m, backoff)

		select {
		case <-opts.Stop:
			return
		case <-deps.Clock.After(backoff.Next()):
		}
	}
}

// runOnce starts the shim once, waits for it to exit, and resets backoff if
// it ran long enough to be considered stable (StableAfter).
func runOnce(deps Deps, opts Options, m resolvedMode, backoff *childbackoff.Backoff) {
	startedAt := deps.Clock.Now()

	stdout := logwriter.New(logPrefix, deps.Log)
	stderr := logwriter.New(logPrefix, deps.Log)
	args := runArgs(m)
	pid, err := deps.StartProcess(opts.BinaryPath, args, runEnv(m), stdout, stderr)
	if err != nil {
		deps.Log("tailscale-funnel: starting failed: %v", err)
		return
	}
	deps.Log("tailscale-funnel: started (pid %d): %s %s", pid, opts.BinaryPath, strings.Join(args, " "))

	status, err := deps.Wait(pid)
	_ = stdout.Close()
	_ = stderr.Close()
	ran := deps.Clock.Now().Sub(startedAt)
	if err != nil {
		deps.Log("tailscale-funnel: supervising pid %d failed: %v", pid, err)
	} else {
		deps.Log("tailscale-funnel: pid %d exited with status %d after %s", pid, status, ran)
	}

	if ran >= StableAfter {
		backoff.Reset()
	}
}
