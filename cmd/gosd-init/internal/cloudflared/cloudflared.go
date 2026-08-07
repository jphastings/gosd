// Package cloudflared implements gosd-init's supervision of a baked-in
// cloudflared binary, exposing a device's [ingress.cloudflared]-declared
// HTTP service to the public internet over a Cloudflare Tunnel with zero
// app code (see epic gosd-virc). v1 supports only locally-managed tunnels:
// gosd.toml declares a tunnel token, a public hostname, and a local port,
// and Run decodes the token, synthesizes cloudflared's own config files
// under /run/gosd/cloudflared/, and keeps the process alive for the life of
// the device.
//
// This package amends the "gosd-init has no interactive surface" decision
// with a narrow, JP-approved carve-out (gosd-oyhi): gosd-SHIPPED system
// services, unlike USER externals, may be gosd-init-supervised. cloudflared
// itself stays consistent with the rest of that decision regardless — it is
// outbound-only (QUIC to the Cloudflare edge) and routes solely to the one
// declared local port, so gosd-init gains no listener and nothing on the
// device becomes remotely reachable except through the tunnel the operator
// explicitly configured.
//
// Following the style established by boot, netup, wifiup, timesync, and
// mdnsresponder, every side-effecting dependency (starting the process,
// waiting for it to exit, the gate marker files, the filesystem, the clock,
// logging) sits behind Deps, so mode resolution, gating, and supervision
// are fully unit-tested with fakes on any OS. platform.go's StartProcess is
// a plain os/exec wrapper needing no "linux" build tag at all (see its doc
// comment) — unlike boot/netup/wifiup/timesync, this package has no
// platform_linux.go/platform_other.go split.
//
// This module ships UNWIRED: nothing in cmd/gosd-init/main.go calls Run
// yet. Wiring Run into the boot sequence — including assigning
// Deps.StartProcess to this package's own StartProcess and Deps.Wait to the
// PID-1 reaper's Wait (see Deps.Wait's doc comment) — is a later bean in
// the same epic, once this package has been reviewed in isolation.
package cloudflared

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/jphastings/gosd/internal/gosdtoml"
)

// Runtime paths, per the bean's locked decision. RuntimeDir is created
// (mode 0700) by writeRuntimeFiles before CredentialsPath and ConfigPath
// (mode 0600) are written into it; both live on the in-memory /run
// tmpfs, never on persistent storage, since they're fully reconstructible
// from gosd.toml on every boot.
const (
	RuntimeDir      = "/run/gosd/cloudflared"
	CredentialsPath = RuntimeDir + "/credentials.json"
	ConfigPath      = RuntimeDir + "/config.yml"
)

// Default tuning values for production wiring; tests use their own, much
// shorter values so they don't take real wall-clock time to run.
const (
	// DefaultNetworkUpPollInterval is how often Run polls for the
	// network-up marker while waiting for it to appear, mirroring
	// timesync.DefaultNetworkUpPollInterval.
	DefaultNetworkUpPollInterval = 2 * time.Second

	// DefaultTimeSyncedTimeout is how long Run waits for the time-synced
	// marker before giving up and starting cloudflared anyway (locked
	// decision): the build-timestamp clock floor keeps TLS validity checks
	// mostly correct even before NTP has landed, and cloudflared's own
	// connection retries (plus this package's backoff) absorb the rest.
	DefaultTimeSyncedTimeout = 2 * time.Minute

	// DefaultTimeSyncedPollInterval is how often Run polls for the
	// time-synced marker while waiting for it, or for DefaultTimeSyncedTimeout
	// to elapse.
	DefaultTimeSyncedPollInterval = 2 * time.Second
)

// runArgs is cloudflared's fixed argv (locked decision): --no-autoupdate
// always (gosd controls the binary's lifecycle via artifact releases, not
// cloudflared's own updater), --loglevel warn (info floods the 115200
// serial console), and --config pointing at the config.yml Run renders.
var runArgs = []string{"tunnel", "--no-autoupdate", "--loglevel", "warn", "--config", ConfigPath, "run"}

// runEnv is cloudflared's fixed environment (locked decision): HOME so
// cloudflared's own ~/.cloudflared probing resolves to somewhere writable
// instead of a nonexistent home directory.
var runEnv = []string{"HOME=" + RuntimeDir}

// Deps bundles every dependency Run needs. Production wiring (the later
// wiring bean's main.go) supplies real implementations; tests supply fakes.
type Deps struct {
	// StartProcess launches path with args and env, directing its
	// stdout/stderr, and returns its pid without waiting for it to exit.
	// Production wires this to this package's own StartProcess
	// (platform.go). This is a package-local seam rather than a shared
	// boot.AppStarter because boot.AppStarter.Start takes no argv at all
	// (it only ever launches /app) — cloudflared needs one.
	StartProcess func(path string, args, env []string, stdout, stderr io.Writer) (pid int, err error)

	// Wait blocks until pid has exited, returning its exit status.
	// Production wires this to the PID-1 reaper's Wait
	// (boot.Platform.Reaper.Wait) — NEVER exec.Cmd.Wait. As PID 1,
	// gosd-init reaps every child (including this one) through a single
	// central wait4(-1, ...) loop (boot's linuxReaper); calling
	// exec.Cmd.Wait directly on the *exec.Cmd StartProcess creates
	// internally would race that loop for the same child's exit status,
	// exactly the hazard boot.AppStarter's doc comment already documents
	// for /app.
	Wait func(pid int) (status int, err error)

	// NetworkUp reports whether the network-up marker file
	// (netup.DefaultNetworkUpPath in production) currently exists. Run
	// polls this (via waitForNetworkUp) before writing any runtime file
	// and again before every restart attempt in the supervise loop — paths
	// are injected rather than importing netup, following timesync's
	// precedent of no cross-package imports between gosd-init's
	// side-effecting feature modules.
	NetworkUp func() (bool, error)

	// TimeSynced reports whether the time-synced marker file
	// (timesync.DefaultTimeSyncedPath in production) currently exists. Run
	// waits for it (via waitForTimeSynced) up to Options.TimeSyncedTimeout
	// before proceeding regardless, since TLS to the Cloudflare edge wants
	// a roughly-correct clock but must not block ingress forever if NTP is
	// unreachable.
	TimeSynced func() (bool, error)

	// MkdirAll and WriteFile create RuntimeDir and write CredentialsPath /
	// ConfigPath into it. Matching os.MkdirAll's and os.WriteFile's exact
	// signatures lets production wiring assign those functions directly.
	MkdirAll  func(path string, perm os.FileMode) error
	WriteFile func(path string, data []byte, perm os.FileMode) error

	Clock Clock

	// NewBackoff creates the restart backoff used by the supervise loop. A
	// func rather than a shared *Backoff so that if Run is ever invoked
	// more than once its restart sequence always starts fresh, mirroring
	// timesync.Deps.NewBackoff's rationale.
	NewBackoff func() *Backoff

	// Log records what cloudflared supervision is doing. Every line this
	// package logs is prefixed "cloudflared: ", following mdnsresponder's
	// "mdns: " precedent. Never nil in production (wired to boot's console
	// logger). Env is never logged, even though runEnv carries no secret
	// today — the token itself never appears in argv or env at all, only
	// in CredentialsPath — because gosd-init's app-launch logging never
	// logs env on principle (many apps' env does carry secrets).
	Log func(format string, args ...any)
}

// Options holds the per-boot behavior knobs for cloudflared supervision.
type Options struct {
	// BinaryPath is where gosd build baked the cloudflared binary for this
	// board. Unused unless Config.Configured() and Baked are both true.
	BinaryPath string

	// Baked reports whether gosd build --ingress cloudflared baked a
	// cloudflared binary into this image at BinaryPath. Wired from a
	// future config.json bit by the later wiring bean; resolveMode never
	// probes the filesystem for the binary itself (locked decision: the
	// "is it baked" bit lives in config.json, not on disk).
	Baked bool

	// Config is the already-parsed [ingress.cloudflared] section of
	// gosd.toml.
	Config gosdtoml.IngressCloudflared

	NetworkUpPollInterval  time.Duration
	TimeSyncedTimeout      time.Duration
	TimeSyncedPollInterval time.Duration

	// Stop, if non-nil, ends supervision when closed. Production leaves
	// this nil so Run runs for the life of the process, as PID 1 requires;
	// tests set it to bound the otherwise-infinite loops.
	Stop <-chan struct{}
}

// Run resolves whether cloudflared should run at all (see resolveMode),
// logging the one actionable line and returning immediately if not, then
// waits for the network-up marker (parking forever if it never appears),
// waits up to Options.TimeSyncedTimeout for the time-synced marker
// (proceeding with a warning if it times out), writes credentials.json and
// config.yml, and supervises cloudflared for as long as Options.Stop isn't
// closed.
//
// Run is meant to be launched in its own goroutine (see
// boot.Deps.StartNetworking's sibling calls): per the "no interactive
// surface" / never-block-/app decision, ingress bring-up must never delay
// anything else gosd-init does.
func Run(deps Deps, opts Options) {
	m := resolveMode(opts.Config, opts.Baked)
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

	if err := writeRuntimeFiles(deps, m); err != nil {
		deps.Log("cloudflared: writing runtime configuration failed: %v", err)
		return
	}

	supervise(deps, opts)
}

// waitForNetworkUp polls deps.NetworkUp (checking immediately, then every
// opts.NetworkUpPollInterval) until it reports true or opts.Stop closes. It
// returns false only in the latter case. Mirrors
// timesync.waitForNetworkUp exactly.
func waitForNetworkUp(deps Deps, opts Options) bool {
	for {
		up, err := deps.NetworkUp()
		if err != nil {
			deps.Log("cloudflared: checking network-up marker failed: %v", err)
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
			deps.Log("cloudflared: checking time-synced marker failed: %v", err)
		} else if synced {
			return
		}

		if !deps.Clock.Now().Before(deadline) {
			deps.Log("cloudflared: time sync did not complete within %s; starting cloudflared anyway (this build's baked timestamp keeps TLS validity mostly correct)", opts.TimeSyncedTimeout)
			return
		}

		select {
		case <-opts.Stop:
			return
		case <-deps.Clock.After(opts.TimeSyncedPollInterval):
		}
	}
}

// writeRuntimeFiles creates RuntimeDir and writes CredentialsPath and
// ConfigPath into it, per the locked 0700/0600 permissions.
func writeRuntimeFiles(deps Deps, m resolvedMode) error {
	if err := deps.MkdirAll(RuntimeDir, 0o700); err != nil {
		return fmt.Errorf("creating %s: %w", RuntimeDir, err)
	}
	if err := deps.WriteFile(CredentialsPath, credentialsJSON(m), 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", CredentialsPath, err)
	}
	if err := deps.WriteFile(ConfigPath, configYAML(m), 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", ConfigPath, err)
	}
	return nil
}

// supervise starts cloudflared and restarts it with backoff whenever it
// exits, for as long as opts.Stop isn't closed. Every dependency is
// injected so this is unit-testable without real processes, clocks, or
// sleeps — the same shape as boot.Supervisor, with one addition: the
// network-up marker is re-checked (parking, not backing off, if it's gone)
// before every single start attempt, not just the first. Locked decision:
// cloudflared holds four redundant edge connections and reconnects itself
// on a network blip, so if it exits anyway, restarting it into a network
// that's still down would only burn backoff for no reason — parking here
// instead means a restart follows the network coming back up almost
// immediately, at the cost of never restarting purely "because the network
// changed" the way, say, mdnsresponder does.
func supervise(deps Deps, opts Options) {
	backoff := deps.NewBackoff()

	for {
		if !waitForNetworkUp(deps, opts) {
			return
		}

		runOnce(deps, opts, backoff)

		select {
		case <-opts.Stop:
			return
		case <-deps.Clock.After(backoff.Next()):
		}
	}
}

// runOnce starts cloudflared once, waits for it to exit, and resets backoff
// if it ran long enough to be considered stable (StableAfter).
func runOnce(deps Deps, opts Options, backoff *Backoff) {
	startedAt := deps.Clock.Now()

	stdout := newLineWriter(deps.Log)
	stderr := newLineWriter(deps.Log)
	pid, err := deps.StartProcess(opts.BinaryPath, runArgs, runEnv, stdout, stderr)
	if err != nil {
		deps.Log("cloudflared: starting failed: %v", err)
		return
	}
	deps.Log("cloudflared: started (pid %d): %s %s", pid, opts.BinaryPath, strings.Join(runArgs, " "))

	status, err := deps.Wait(pid)
	_ = stdout.Close()
	_ = stderr.Close()
	ran := deps.Clock.Now().Sub(startedAt)
	if err != nil {
		deps.Log("cloudflared: supervising pid %d failed: %v", pid, err)
	} else {
		deps.Log("cloudflared: pid %d exited with status %d after %s", pid, status, ran)
	}

	if ran >= StableAfter {
		backoff.Reset()
	}
}
