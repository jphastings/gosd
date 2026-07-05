// Package mdnsresponder implements the gosd-init mDNS responder: answering
// A/AAAA queries for <hostname>.local on every currently-up network
// interface, using the locked github.com/pion/mdns/v2 dependency (see
// gosd-r796).
//
// gosd-init is the only mDNS responder the platform runs — apps get no
// interactive surface at all (see CLAUDE.md), so there's no risk of a user
// app fighting gosd-init over the <hostname>.local A/AAAA records
// themselves. If a user app later wants to advertise its own services over
// mDNS/zeroconf (e.g. _http._tcp), it must use a distinct service instance
// name from the device hostname: gosd-init owns <hostname>.local exclusively,
// and nothing here coordinates with an app-level responder sharing port 5353.
//
// Unlike netup/wifiup/timesync's Linux-syscall-bound platform code,
// pion/mdns is pure Go over plain UDP multicast sockets, so the production
// responder in server.go needs no platform_linux.go/platform_other.go split
// and no "linux" build tag at all — it compiles, and can genuinely run, on
// macOS too (see server_test.go, which exercises it against real sockets,
// guarded so an environment that can't join multicast groups skips rather
// than flakes). Only the restart-on-change loop in this file is unit tested
// with a fake Server/NewServerFunc, following the style established by
// netup/wifiup/timesync of keeping side-effecting dependencies behind thin
// interfaces.
package mdnsresponder

// Deps bundles every dependency the responder restart loop needs.
// Production wiring (main.go) supplies NewServer (this package's real,
// pion/mdns-backed implementation); tests supply a fake.
type Deps struct {
	// NewServer starts a new responder answering as hostname+".local" on
	// every interface that's up at the moment it's called. A func rather
	// than a shared, mutable connection because pion/mdns's Conn has no
	// API to add/remove interfaces or change its LocalNames after
	// construction — restarting is the whole strategy (see Run's doc
	// comment on why this is an acceptable simplification of "re-announce
	// on address change").
	NewServer NewServerFunc

	// Changed fires whenever the set of up interfaces or their addresses
	// may have changed. Production wires this to netup/wifiup's existing
	// MarkNetworkUp/ClearNetworkUp hooks (main.go wraps those closures,
	// rather than netup/wifiup gaining any new API of their own) — see
	// main.go's netupDeps/wifiupDeps for the wiring. A *Signal rather than
	// a plain channel so bursts of changes (e.g. several link events in
	// quick succession) coalesce into a single restart instead of queuing
	// one per event.
	Changed <-chan struct{}

	// Log records what the responder is doing. Never nil in production
	// (wired to boot's console logger).
	Log func(format string, args ...any)
}

// Options holds the per-boot behavior knobs for the mDNS responder.
type Options struct {
	// Hostname is the device's resolved hostname (config.json, cmdline,
	// and gosd.toml overrides already applied) — Run answers for
	// Hostname+".local".
	Hostname string

	// Stop, if non-nil, ends the responder when closed. Production leaves
	// this nil so it runs for the life of the process, as PID 1 requires;
	// tests set it to bound the otherwise-infinite watch loop.
	Stop <-chan struct{}
}

// Run starts the mDNS responder and keeps it answering for opts.Hostname+
// ".local" for as long as opts.Stop isn't closed, restarting it (via
// deps.NewServer) every time deps.Changed fires.
//
// The bean this implements (gosd-r796) asks for the responder to be
// "re-announced/restarted on address change". pion/mdns's Conn has no API
// to add a newly-appeared interface or update its answers after
// construction, so a full stop/start of the underlying responder — rather
// than an in-place re-announcement — is the mechanism here; this is
// documented as an accepted simplification rather than a deviation to fix
// later, since the bean explicitly allows it. A brief gap in answering
// mDNS queries during a restart (milliseconds: closing old UDP sockets and
// opening new ones) is invisible to any realistic client, which retries
// mDNS queries on no answer as a matter of course.
//
// Run is meant to be launched in its own goroutine (see
// boot.Deps.StartNetworking): per the locked behavior, mDNS bring-up must
// never block or delay /app's start.
func Run(deps Deps, opts Options) {
	var current Server
	defer func() {
		if current != nil {
			if err := current.Close(); err != nil {
				deps.Log("mdns: closing responder failed: %v", err)
			}
		}
	}()

	restart := func() {
		if current != nil {
			if err := current.Close(); err != nil {
				deps.Log("mdns: closing previous responder failed: %v", err)
			}
			current = nil
		}
		srv, err := deps.NewServer(opts.Hostname)
		if err != nil {
			// Expected immediately at boot, before any interface is up:
			// no interfaces means pion/mdns has nothing to bind to. The
			// next network-change notification (the first successful
			// DHCP lease, almost always) retries automatically.
			deps.Log("mdns: no responder yet for %s.local (%v); will retry on the next network change", opts.Hostname, err)
			return
		}
		current = srv
		deps.Log("mdns: answering as %s.local on all up interfaces", opts.Hostname)
	}

	restart()

	for {
		select {
		case <-opts.Stop:
			return
		case <-deps.Changed:
			deps.Log("mdns: network changed; restarting responder")
			restart()
		}
	}
}
