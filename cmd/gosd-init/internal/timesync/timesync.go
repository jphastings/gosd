// Package timesync implements SNTP time synchronization for gosd-init.
// Neither board has a battery-backed RTC, so the clock starts at the Unix
// epoch on every boot and must be corrected before anything relying on
// wall-clock time — most importantly TLS/x509 certificate validity
// checks — can be trusted.
//
// Following the style established by boot, netup, and wifiup, every
// side-effecting dependency (the NTP client, settimeofday, the clock,
// marker file I/O) sits behind a thin interface, so the retry/refresh
// state machine in this file is fully unit-tested with fakes on any OS.
// The real settimeofday-backed SystemClock lives in platform_linux.go
// behind a "linux" build tag; platform_other.go stubs it out so `go test
// ./...` still passes on macOS. NTPClient's real implementation
// (ntpclient.go) needs no such gating: querying an NTP server is a plain
// UDP round-trip, not a Linux-specific syscall.
package timesync

import "time"

// Default tuning values for production wiring (main.go); tests use their
// own, much shorter values so they don't take real wall-clock time to run.
const (
	// DefaultResyncInterval is how often Run re-queries the NTP servers
	// after the first successful sync, per the bean's "re-sync hourly"
	// instruction.
	DefaultResyncInterval = time.Hour

	// DefaultNetworkUpPollInterval is how often Run polls for the
	// network-up marker file while waiting for it to appear. There's no
	// inotify inside the initramfs, and none is needed: noticing the
	// marker within a couple of seconds of it appearing is more than
	// good enough.
	DefaultNetworkUpPollInterval = 2 * time.Second

	// DefaultMaxStep is the post-first-sync step guard's threshold
	// (Options.MaxStep), matching ntpd's classic ~1000s step/panic
	// threshold — see stepGuard's doc for the full design (gosd-0esw).
	DefaultMaxStep = 1000 * time.Second
)

// DefaultServers is the NTP server list used when config.json doesn't
// specify one, per the bean.
var DefaultServers = []string{"pool.ntp.org"}

// Deps bundles every dependency the time-sync state machine needs.
// Production wiring (main.go) supplies Platform's real implementations;
// tests supply fakes.
type Deps struct {
	NTP    NTPClient
	System SystemClock
	Clock  Clock

	// NewBackoff creates the retry/backoff strategy used until the first
	// sync succeeds. A func rather than a shared *Backoff so a restart of
	// the retry loop always starts its sequence from scratch.
	NewBackoff func() *Backoff

	// NetworkUp reports whether the network-up marker file
	// (netup.DefaultNetworkUpPath in production) currently exists. Run
	// polls this with Clock before attempting any NTP query, per the
	// bean's "watch /run/gosd/network-up" instruction.
	NetworkUp func() (bool, error)

	// MarkTimeSynced creates the /run/gosd/time-synced marker file
	// (DefaultTimeSyncedPath in production) on the first successful sync.
	MarkTimeSynced func() error

	// Log records what the time-sync state machine is doing. Never nil
	// in production (wired to boot's console logger).
	Log func(format string, args ...any)
}

// Options holds the per-boot behavior knobs for time sync.
type Options struct {
	// Servers is the ordered list of NTP servers to try each round, tried
	// in order until one answers. config.json's optional ntpServers
	// field, defaulting to DefaultServers.
	Servers []string

	// ResyncEvery is how often Run re-queries the server list after the
	// first successful sync.
	ResyncEvery time.Duration

	// NetworkUpPollInterval is how often Run polls NetworkUp while
	// waiting for the marker to appear.
	NetworkUpPollInterval time.Duration

	// Stop, if non-nil, ends time sync when closed. Production leaves
	// this nil so it runs for the life of the process, as PID 1
	// requires; tests set it to bound the otherwise-infinite loops.
	Stop <-chan struct{}

	// Floor is the earliest time Run will ever accept from an NTP
	// server; any result before it is refused and logged, exactly like a
	// query that gets no answer at all (see checkFloor). Production
	// wires this to config.json's baked build timestamp
	// (initcfg.Config.BuildTime) — see the package doc and gosd-0esw. The
	// zero time.Time (the default) disables the check entirely, which is
	// also what a config.json baked before that field existed parses to.
	Floor time.Time

	// MaxStep bounds how large a step a resync — never the first sync,
	// see stepGuard's doc — may apply outright. A candidate whose
	// distance from where the clock should currently read exceeds
	// MaxStep is refused and logged, applied only once an immediately
	// following resync reports a value that agrees with it. Zero (the
	// default) disables the guard. Production wires this to
	// DefaultMaxStep.
	MaxStep time.Duration
}

// Run waits for the network-up marker, then synchronizes the system
// clock via SNTP: retrying with backoff until the first successful sync
// (writing the time-synced marker and logging the step change once it
// lands), then re-querying every opts.ResyncEvery for as long as
// opts.Stop isn't closed.
//
// Run is meant to be launched in its own goroutine (see
// boot.Deps.StartNetworking): per the locked behavior, time sync must
// never block or delay /app's start.
func Run(deps Deps, opts Options) {
	if !waitForNetworkUp(deps, opts) {
		return
	}

	// guard is created before the first sync (not just before the resync
	// loop) so that first sync anchors it — see stepGuard's doc: every
	// resync's step is measured against wherever the clock should be
	// given the most recently *applied* sync, and the first sync is the
	// only source of that anchor before any resync has ever run.
	guard := &stepGuard{}
	if !syncUntilSuccess(deps, opts, guard) {
		return
	}

	for {
		select {
		case <-opts.Stop:
			return
		case <-deps.Clock.After(opts.ResyncEvery):
			guard.resync(deps, opts)
		}
	}
}

// waitForNetworkUp polls deps.NetworkUp (checking immediately, then every
// opts.NetworkUpPollInterval) until it reports true or opts.Stop closes.
// It returns false only in the latter case.
func waitForNetworkUp(deps Deps, opts Options) bool {
	for {
		up, err := deps.NetworkUp()
		if err != nil {
			deps.Log("checking network-up marker failed: %v", err)
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

// syncUntilSuccess queries opts.Servers, retrying the whole list with
// backoff on failure, until one round produces a result that clears
// opts.Floor (see checkFloor) or opts.Stop closes. A pre-floor result is
// treated exactly like a round with no answer at all: logged and retried
// after backoff, never applied. On a result that does clear the floor, it
// sets the system clock, logs the step change, anchors guard for the
// resync loop to measure steps against (see stepGuard's doc), writes the
// time-synced marker, and returns true. It returns false only if
// opts.Stop closed first.
func syncUntilSuccess(deps Deps, opts Options, guard *stepGuard) bool {
	backoff := deps.NewBackoff()
	for {
		if newTime, ok := queryServers(deps, opts.Servers); ok && checkFloor(deps, opts, newTime) {
			old := deps.Clock.Now()
			stepClock(deps, old, newTime)
			guard.setAnchor(old, newTime)
			if err := deps.MarkTimeSynced(); err != nil {
				deps.Log("writing time-synced marker failed: %v", err)
			}
			return true
		}

		delay := backoff.Next()
		deps.Log("NTP sync failed on every configured server; retrying in %s", delay)
		select {
		case <-opts.Stop:
			return false
		case <-deps.Clock.After(delay):
		}
	}
}

// resync re-queries opts.Servers once — no backoff, since the next
// attempt is only opts.ResyncEvery away regardless — applying and
// logging the adjustment once a result clears both guards: the build
// floor (checkFloor) and, unlike the first sync, g's max-step
// confirmation (stepGuard.check). Any refusal, and any round where no
// server answered, is just logged so the next scheduled resync tries
// again.
func (g *stepGuard) resync(deps Deps, opts Options) {
	newTime, ok := queryServers(deps, opts.Servers)
	if !ok {
		deps.Log("scheduled NTP resync failed on every configured server; will retry at the next resync")
		return
	}
	if !checkFloor(deps, opts, newTime) {
		return
	}

	old := deps.Clock.Now()
	if !g.check(deps, opts, old, newTime) {
		return
	}
	stepClock(deps, old, newTime)
	g.setAnchor(old, newTime)
}

// checkFloor reports whether newTime clears opts.Floor, logging and
// refusing otherwise. The zero Floor (config.json baked before gosd-0esw,
// or before the build-timestamp field existed) disables the check
// entirely — see Options.Floor's doc.
func checkFloor(deps Deps, opts Options, newTime time.Time) bool {
	if opts.Floor.IsZero() || !newTime.Before(opts.Floor) {
		return true
	}
	deps.Log("NTP result %s is before this build's floor of %s; refusing (a forged or misbehaving server?)",
		newTime.Format(time.RFC3339), opts.Floor.Format(time.RFC3339))
	return false
}

// queryServers tries each server in order, returning the first one that
// answers. NTP servers (especially pool.ntp.org, a shared round-robin
// pool) are occasionally slow or briefly unreachable; trying the whole
// list before giving up the round means one flaky server doesn't cost an
// entire backoff cycle.
func queryServers(deps Deps, servers []string) (time.Time, bool) {
	for _, server := range servers {
		t, err := deps.NTP.Query(server)
		if err != nil {
			deps.Log("querying NTP server %s failed: %v", server, err)
			continue
		}
		return t, true
	}
	return time.Time{}, false
}

// stepClock sets the system clock to newTime and logs the step change
// (old -> new), per the bean. old is the deps.Clock reading immediately
// before the step, passed in by the caller (rather than read again here)
// so a resync's step-guard measurement and the logged "old" value are
// exactly the same reading.
func stepClock(deps Deps, old, newTime time.Time) {
	if err := deps.System.Set(newTime); err != nil {
		deps.Log("setting system clock failed: %v", err)
		return
	}
	deps.Log("system clock synchronized via NTP: %s -> %s", old.Format(time.RFC3339), newTime.Format(time.RFC3339))
}
