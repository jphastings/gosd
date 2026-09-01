// Package wifiup brings up WiFi networking after /app has already been
// launched: waiting for the wlan interface to appear (firmware for
// brcmfmac and similar chipsets loads at driver probe, which can take
// seconds), associating with an open or WPA2-PSK network via nl80211
// (no wpa_supplicant — brcmfmac's firmware SME handles the 4-way
// handshake once given the PMK), reconnecting on deauth/disconnect, and
// handing the interface to netup.RunDHCP once associated.
//
// Following the style established by boot and netup, every
// side-effecting dependency (nl80211, the DHCPv4 client, the clock, file
// writes) sits behind a thin interface, so the association/reconnect
// state machine in this file is fully unit-tested with fakes on any OS.
// The real, nl80211-backed WifiClient implementation lives in
// platform_linux.go behind a "linux" build tag; platform_other.go stubs
// it out so `go test ./...` still passes on macOS.
package wifiup

import (
	"context"
	"net"
	"strings"
	"time"

	"github.com/jphastings/gosd/cmd/gosd-init/internal/netup"
)

// Deps bundles every dependency the WiFi state machine needs. Production
// wiring (main.go) supplies the real implementations; tests supply fakes.
type Deps struct {
	Wifi        WifiClient
	Credentials CredentialSource

	// Links, DHCP, Clock and NewBackoff are handed straight to
	// netup.RunDHCP once associated (DHCP itself doesn't care whether the
	// underlying medium is wired or wireless), and Links is also used
	// directly to bring the wlan interface up and to apply a lease once
	// obtained — the same shape netup.Run itself uses for wired
	// interfaces.
	Links      netup.Links
	DHCP       netup.DHCPClient
	Clock      netup.Clock
	NewBackoff func() *netup.Backoff

	WriteResolvConf func(dns []net.IP) error
	// MarkNetworkUp and ClearNetworkUp report ifi as up or down. This is
	// the same shared, refcounted marker netup.Deps uses — see that
	// type's doc for why (bean gosd-akk4): production wiring (main.go)
	// routes both packages through one netup.UpSet keyed by interface
	// name, so WiFi going down never clobbers a still-up Ethernet link
	// (or vice versa) on a dual-interface board.
	MarkNetworkUp  func(iface string) error
	ClearNetworkUp func(iface string) error

	// RegisterSecret registers a runtime join request's passphrase for
	// crash-report redaction the moment the request is read, alongside
	// boot/sequence.go's existing STA rule (epic gosd-ojbm decision 7) —
	// belt and suspenders for a request that reached /run/gosd/wifi
	// without going through wifi.Join's own fault.RegisterSecretString
	// call. nil skips registration (tests that don't care).
	RegisterSecret func(secret, label string)

	// Persist writes a successful runtime join's ssid/passphrase into the
	// card's config tree, via the same write path cloud-init seed
	// consumption uses (epic decision 3). Called only when the request
	// asked for it (Persist: true) and only after the join succeeds — never
	// on failure. nil means persistence is unavailable; a request asking
	// for it logs a failure and still reports the join itself as joined.
	Persist func(ssid, passphrase string) error

	// RestartIngress fires the ingress restart signal(s) after every
	// successful runtime join, including a same-SSID one (epic decision
	// 4). nil means no ingress is wired on this image.
	RestartIngress func()

	Log func(format string, args ...any)
}

// Options holds the per-boot behavior knobs for WiFi bring-up.
type Options struct {
	// Stop, if non-nil, ends WiFi bring-up when closed. Production leaves
	// this nil so it runs for the life of the process, as PID 1 requires;
	// tests set it to bound the otherwise-infinite loops.
	Stop <-chan struct{}

	// RequestDir is where the runtime-join request/status protocol
	// (internal/wifictl) lives. Empty disables the watcher entirely — the
	// zero value every pre-existing test gets, so tests that don't
	// exercise runtime join never touch a filesystem for it; production
	// wires this to wifictl.Dir (epic gosd-ojbm decision 5).
	RequestDir string

	// RequestPollInterval overrides the watcher's poll cadence. Zero uses
	// DefaultRequestPollInterval.
	RequestPollInterval time.Duration
}

// Run waits for credentials and a wlan interface, then associates and
// maintains the connection (reconnecting on deauth/disconnect and after
// scan/connect failures) for as long as opts.Stop isn't closed. It never
// blocks /app's start: like netup.Run, it's meant to be launched in its
// own goroutine.
//
// The runtime-join request watcher (see watchRequests) always runs
// alongside the association loop whenever opts.RequestDir is set — even
// when no WiFi credentials are configured at boot, and even when deps.Wifi
// is nil (no WiFi hardware/driver at all) — so an app's wifi.Join call
// gets an honest joined/failed answer rather than a silent hang (epic
// gosd-ojbm decisions 5 and 8). With deps.Wifi nil, Run does nothing beyond
// running that watcher, which fails every request "no WiFi interface".
func Run(deps Deps, opts Options) {
	state := newCredState()

	if creds, ok, err := deps.Credentials.Credentials(); err != nil {
		deps.Log("reading WiFi credentials failed: %v", err)
	} else if ok && creds.Unsupported != "" {
		deps.Log("WiFi network %q requires %s, which gosd-init does not support (WPA2-PSK and open networks only); skipping WiFi bring-up", creds.SSID, creds.Unsupported)
	} else if ok {
		state.set(creds, nil)
	} else {
		deps.Log("no WiFi credentials configured; the runtime join watcher is still running")
	}

	if opts.RequestDir != "" {
		go watchRequests(deps, opts, state)
	}

	if deps.Wifi == nil {
		deps.Log("no WiFi interface available; skipping WiFi bring-up")
		<-opts.Stop
		return
	}

	for {
		creds, ok, outcome, changed := state.current()
		if !ok {
			select {
			case <-opts.Stop:
				return
			case <-changed:
				continue
			}
		}

		ifi, ok := waitForInterface(deps, opts.Stop)
		if !ok {
			return // opts.Stop closed before a wlan interface appeared.
		}
		if !creds.Open {
			ifi = skipWithoutOffloadedHandshake(deps, ifi)
		}
		deps.Log("using WiFi interface %s", ifi.Name)

		if err := deps.Links.SetUp(ifi.Name); err != nil {
			deps.Log("bringing up %s failed: %v", ifi.Name, err)
		}

		runAssociationLoop(deps, ifi, creds, mergedStop(opts.Stop, changed), outcome)

		select {
		case <-opts.Stop:
			return
		default:
			// changed fired: a runtime join request replaced the current
			// creds. Loop again and pick them up (epic decision 6).
		}
	}
}

// waitForInterface polls for a wlan-station interface with backoff,
// patiently, since firmware for the WiFi chipset loads asynchronously at
// driver probe and can take several seconds after gosd-init starts. It
// returns false only if opts.Stop closes first.
func waitForInterface(deps Deps, stop <-chan struct{}) (Interface, bool) {
	backoff := deps.NewBackoff()
	for {
		ifis, err := deps.Wifi.Interfaces()
		if err != nil {
			deps.Log("listing WiFi interfaces failed: %v", err)
		} else if ifi, found := pickInterface(ifis); found {
			return ifi, true
		}

		delay := backoff.Next()
		select {
		case <-stop:
			return Interface{}, false
		case <-deps.Clock.After(delay):
		}
	}
}

// pickInterface prefers a "wlan"-prefixed interface (the near-universal
// naming for Linux WiFi station interfaces, including brcmfmac), falling
// back to the first interface reported if none matches — gosd's target
// boards each have exactly one onboard WiFi radio, so any station
// interface that exists is the one to use.
func pickInterface(ifis []Interface) (Interface, bool) {
	for _, ifi := range ifis {
		if strings.HasPrefix(ifi.Name, "wlan") {
			return ifi, true
		}
	}
	if len(ifis) > 0 {
		return ifis[0], true
	}
	return Interface{}, false
}

// skipWithoutOffloadedHandshake guards a WPA2-PSK join before the first
// CONNECT: a phy without NL80211_EXT_FEATURE_4WAY_HANDSHAKE_STA_PSK has
// every PMK-carrying CONNECT rejected with EINVAL by the kernel itself,
// so retrying against it can never succeed — and on kernels that build
// in mac80211_hwsim, the picked interface may be one of its simulated
// radios rather than the real one (bean gosd-6nl2: pi-zero-w spent a
// bench session in exactly that EINVAL loop). If picked lacks the
// feature this logs an actionable error and moves to the next capable
// candidate; with no capable candidate it returns picked anyway, so the
// association loop still runs (and keeps logging honestly) rather than
// silently giving up. A failed check is treated as "unknown", not as
// unsupported — never skipping a real radio over a netlink hiccup.
func skipWithoutOffloadedHandshake(deps Deps, picked Interface) Interface {
	supported, err := deps.Wifi.SupportsOffloadedHandshake(picked)
	if err != nil {
		deps.Log("checking WPA2 handshake offload on %s failed: %v; proceeding with it anyway", picked.Name, err)
		return picked
	}
	if supported {
		return picked
	}
	deps.Log("%s cannot do firmware-offloaded WPA2-PSK (missing 4WAY_HANDSHAKE_STA_PSK); if this device has multiple WiFi interfaces one may be a phantom (e.g. mac80211_hwsim) — see bean gosd-6nl2", picked.Name)

	ifis, err := deps.Wifi.Interfaces()
	if err != nil {
		return picked
	}
	for _, cand := range ifis {
		if cand.Index == picked.Index {
			continue
		}
		supported, err := deps.Wifi.SupportsOffloadedHandshake(cand)
		if err != nil {
			deps.Log("checking WPA2 handshake offload on %s failed: %v", cand.Name, err)
			continue
		}
		if !supported {
			deps.Log("%s cannot do firmware-offloaded WPA2-PSK (missing 4WAY_HANDSHAKE_STA_PSK); if this device has multiple WiFi interfaces one may be a phantom (e.g. mac80211_hwsim) — see bean gosd-6nl2", cand.Name)
			continue
		}
		return cand
	}
	return picked
}

// runAssociationLoop associates ifi with creds's network, retrying
// forever with backoff on failure (the AP may be down at boot), and runs
// DHCP for as long as the association holds. It returns only when stop
// is closed.
//
// firstOutcome, if non-nil, is called exactly once — after the FIRST
// associate attempt this call makes settles, joined or not — with the
// joined result and, on failure, the most precise reason available. It
// exists for the runtime-join reconciler (see handleRequest): Run's own
// per-generation stop channel closes and this call returns whenever a new
// request replaces the current creds, so "first attempt of this call" is
// exactly "the attempt the reconciler is reporting a status for" (epic
// gosd-ojbm decision 6 — after that one bounded attempt, the credentials
// stay current and this loop's ordinary retry/reconnect logic owns them
// silently, which is why firstOutcome fires at most once per call).
func runAssociationLoop(deps Deps, ifi Interface, creds Credentials, stop <-chan struct{}, firstOutcome func(joined bool, reason string)) {
	report := firstOutcome
	notify := func(joined bool, reason string) {
		if report == nil {
			return
		}
		report(joined, reason)
		report = nil
	}

	watcher, err := deps.Wifi.WatchDisconnects(ifi)
	if err != nil {
		deps.Log("subscribing to %s disconnect events failed: %v; association losses will be logged without a reason code", ifi.Name, err)
		watcher = nil
	} else {
		defer func() { _ = watcher.Close() }()
	}

	backoff := deps.NewBackoff()
	for {
		select {
		case <-stop:
			return
		default:
		}

		if err := associate(deps, ifi, creds); err != nil {
			notify(false, err.Error())
			delay := backoff.Next()
			deps.Log("associating %s with %q failed: %v; retrying in %s", ifi.Name, creds.SSID, err, delay)
			select {
			case <-stop:
				return
			case <-deps.Clock.After(delay):
				continue
			}
		}
		// The CONNECT ack only means the kernel accepted the request —
		// the firmware's scan/auth/assoc/4-way handshake is still in
		// flight (bean gosd-anyp: logging "associated" here masked a
		// no-op CONNECT for two bench days), and a wrong PSK leaves that
		// handshake stuck forever without the CONNECT call itself ever
		// failing. So the backoff is NOT reset here — only below, once
		// runUntilDisconnect reports the association was actually
		// confirmed at least once. Resetting on the ack alone used to
		// reconnect-storm a wrong-PSK network at a fixed ~3s cadence
		// forever (bean gosd-vcnr): a cycle that never associates now
		// takes backoff.Next() like a failed associate.
		deps.Log("%s: connect accepted for %q; awaiting association", ifi.Name, creds.SSID)
		if watcher != nil {
			// Drop any reason events emitted before or during this
			// (re)connect — most notably associate's own defensive
			// Disconnect — so a stale code is never pinned on the next
			// genuine association loss.
			watcher.TakeReason()
		}

		// notify(true, "") must fire the MOMENT association is first
		// confirmed, not after runUntilDisconnect returns: that only
		// happens once the connection is later LOST (or stop fires) —
		// watchAssociation keeps polling indefinitely for as long as
		// Associated keeps reporting true, by design, since its job is to
		// maintain the connection, not just confirm it once. (notify itself
		// is already a fire-at-most-once guard, so onFirstAssociated can be
		// wired straight to it with no extra bookkeeping here.)
		associated, reason, hasReason := runUntilDisconnect(deps, ifi, watcher, stop, func() { notify(true, "") })
		if associated {
			backoff.Reset()
			continue
		}
		reasonText := "connection was never confirmed (the handshake likely failed or timed out)"
		if hasReason {
			reasonText = reason.String()
		}
		notify(false, reasonText)
		delay := backoff.Next()
		deps.Log("%s: %q accepted the connect but association was never confirmed; retrying in %s", ifi.Name, creds.SSID, delay)
		select {
		case <-stop:
			return
		case <-deps.Clock.After(delay):
		}
	}
}

// associate issues the nl80211 connect for creds: a plain Connect for an
// open network, or ConnectPSK with the already-resolved 256-bit PMK for
// WPA2-PSK — resolved once by CredentialSource.Credentials (either via
// PBKDF2 from a passphrase or decoded directly from a pre-hashed hex
// value), so this call site never needs to know or care which form the
// credential started as.
//
// There is no prior scan step to gate on here at all (for a hidden or a
// broadcasting network alike), so a hidden SSID needs no separate
// directed-scan path — CONNECT already carries the target SSID straight to
// the driver/firmware, which performs its own active/directed probe for
// that exact SSID as part of joining.
func associate(deps Deps, ifi Interface, creds Credentials) error {
	// Disconnect first, unconditionally: on a fresh boot this is a
	// harmless no-op (nothing to disconnect from), but after a lost
	// association or a failed connect attempt it clears any partial or
	// stale nl80211 connection state before retrying, so a driver that's
	// still "trying" a previous BSS doesn't reject the next CONNECT.
	// Errors are expected whenever there was nothing to disconnect and
	// aren't worth logging on every single (re)connect attempt.
	_ = deps.Wifi.Disconnect(ifi)

	if creds.Open {
		return deps.Wifi.Connect(ifi, creds.SSID)
	}
	return deps.Wifi.ConnectPSK(ifi, creds.SSID, creds.PSK)
}

// runUntilDisconnect runs netup.RunDHCP on ifi until either the
// association is lost (detected by polling WifiClient.Associated) or
// stop is closed, then returns so runAssociationLoop can reconnect. The
// returned associated bool reports whether ifi was ever actually confirmed
// associated during the run (see watchAssociation) — false means the
// CONNECT was acked but the handshake never completed (e.g. a wrong
// PSK), which the caller must treat like a failed associate rather than
// resetting its backoff. reason/hasReason carry the most recent nl80211
// disconnect reason observed before the loss was noticed, if any — used by
// runAssociationLoop's firstOutcome reporting. onFirstAssociated, if
// non-nil, is called (from the internal watcher goroutine, synchronized
// with this function's own return via watchDone below) the moment
// Associated is FIRST observed true — since this function otherwise only
// returns on loss or stop, which for a healthy connection can be
// arbitrarily far in the future.
func runUntilDisconnect(deps Deps, ifi Interface, watcher DisconnectWatcher, stop <-chan struct{}, onFirstAssociated func()) (associated bool, reason DisconnectReason, hasReason bool) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	watchDone := make(chan struct{})
	go func() {
		defer close(watchDone)
		associated, reason, hasReason = watchAssociation(deps, ifi, watcher, cancel, stop, onFirstAssociated)
	}()

	ndeps := netup.Deps{
		DHCP:       deps.DHCP,
		Clock:      deps.Clock,
		NewBackoff: deps.NewBackoff,
		Log:        deps.Log,
	}
	if err := netup.RunDHCP(ctx, ndeps, ifi.Name, onLeaseFor(deps, ifi.Name)); err != nil {
		deps.Log("DHCP on %s stopped unexpectedly: %v", ifi.Name, err)
	}

	cancel()
	<-watchDone
	return associated, reason, hasReason
}

// associationPollPeriod is how often watchAssociation checks whether ifi
// is still associated. mdlayher/wifi exposes no deauth/disconnect event
// stream (only request/response nl80211 commands), so polling BSS status
// is the only portable way to detect a lost association; the
// DisconnectWatcher supplements the poll with the reason code once a
// loss is noticed, it doesn't replace loss detection.
const associationPollPeriod = 3 * time.Second

// watchAssociation polls ifi's association state every
// associationPollPeriod and calls disconnect (which cancels the DHCP
// context in runUntilDisconnect) as soon as it's lost, or when stop
// closes — either way, disconnect must always be called exactly once
// before this returns, or runUntilDisconnect would block forever waiting
// on the now-uncancellable DHCP context. The returned associated bool
// reports whether Associated was ever observed true before returning, so
// runUntilDisconnect can tell a connect that was acked but never actually
// associated (e.g. a wrong PSK) apart from a genuine, if brief,
// association. reason/hasReason is whatever disconnect reason the watcher
// most recently observed at the moment the loss was noticed — the same
// value this function's own log line already renders, now also handed back
// to the caller. onFirstAssociated, if non-nil, is called exactly once, the
// moment Associated is first observed true — this function's OWN return
// only happens on loss or stop, which for a healthy connection may never
// come, so a caller that needs to know "confirmed at least once" without
// waiting for that can't rely on the return alone.
func watchAssociation(deps Deps, ifi Interface, watcher DisconnectWatcher, disconnect context.CancelFunc, stop <-chan struct{}, onFirstAssociated func()) (associated bool, reason DisconnectReason, hasReason bool) {
	for {
		select {
		case <-stop:
			disconnect()
			return associated, reason, hasReason
		case <-deps.Clock.After(associationPollPeriod):
		}

		ok, err := deps.Wifi.Associated(ifi)
		if err != nil {
			deps.Log("checking association on %s failed: %v", ifi.Name, err)
			continue
		}
		if ok {
			if !associated && onFirstAssociated != nil {
				onFirstAssociated()
			}
			associated = true
			continue
		}
		reason, hasReason = takeReason(watcher)
		if hasReason {
			deps.Log("%s lost its WiFi association (%s); reconnecting", ifi.Name, reason)
		} else {
			deps.Log("%s lost its WiFi association; reconnecting", ifi.Name)
		}
		// Mirrors netup's link-down teardown (bean gosd-1lx7): once
		// associated is confirmed lost, the address DHCP assigned for
		// the old association is no longer valid, and leaving it
		// would let AddrReplace stack the next lease's address
		// alongside it instead of replacing it.
		if err := deps.Links.FlushAddrs(ifi.Name); err != nil {
			deps.Log("flushing addresses on %s failed: %v", ifi.Name, err)
		}
		if err := deps.ClearNetworkUp(ifi.Name); err != nil {
			deps.Log("clearing network-up marker for %s failed: %v", ifi.Name, err)
		}
		disconnect()
		return associated, reason, hasReason
	}
}

// takeReason consults the watcher when there is one; a nil watcher (the
// subscription failed at loop start) simply never observes a reason.
func takeReason(watcher DisconnectWatcher) (DisconnectReason, bool) {
	if watcher == nil {
		return DisconnectReason{}, false
	}
	return watcher.TakeReason()
}
