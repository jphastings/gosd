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
	MarkNetworkUp   func() error
	ClearNetworkUp  func() error

	Log func(format string, args ...any)
}

// Options holds the per-boot behavior knobs for WiFi bring-up.
type Options struct {
	// Stop, if non-nil, ends WiFi bring-up when closed. Production leaves
	// this nil so it runs for the life of the process, as PID 1 requires;
	// tests set it to bound the otherwise-infinite loops.
	Stop <-chan struct{}
}

// Run waits for credentials and a wlan interface, then associates and
// maintains the connection (reconnecting on deauth/disconnect and after
// scan/connect failures) for as long as opts.Stop isn't closed. It never
// blocks /app's start: like netup.Run, it's meant to be launched in its
// own goroutine.
//
// Run does nothing at all — not even waiting for an interface — when no
// WiFi credentials are configured (empty SSID), so an Ethernet-only board
// never spins a WiFi retry loop.
func Run(deps Deps, opts Options) {
	creds, ok, err := deps.Credentials.Credentials()
	if err != nil {
		deps.Log("reading WiFi credentials failed: %v", err)
		return
	}
	if !ok {
		deps.Log("no WiFi credentials configured; skipping WiFi bring-up")
		return
	}
	if creds.Unsupported != "" {
		deps.Log("WiFi network %q requires %s, which gosd-init does not support (WPA2-PSK and open networks only); skipping WiFi bring-up", creds.SSID, creds.Unsupported)
		return
	}

	ifi, ok := waitForInterface(deps, opts.Stop)
	if !ok {
		return // opts.Stop closed before a wlan interface appeared.
	}
	deps.Log("using WiFi interface %s", ifi.Name)

	if err := deps.Links.SetUp(ifi.Name); err != nil {
		deps.Log("bringing up %s failed: %v", ifi.Name, err)
	}

	if creds.Hidden {
		// A hidden network never appears in a passive scan, so there is
		// no scan-match step to wait on here (see associate): the join
		// goes straight to nl80211 CONNECT with the SSID, which drives
		// an active/directed probe for that SSID as part of the
		// firmware's own join process (verified against brcmfmac's
		// cfg80211_connect handling — see credentials.go's Hidden
		// docstring). This log exists only so a slow join reads as
		// "expected" on the serial console rather than "stuck".
		deps.Log("hidden SSID %q: probing directly; this can take longer", creds.SSID)
	}

	runAssociationLoop(deps, ifi, creds, opts.Stop)
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

// runAssociationLoop associates ifi with creds's network, retrying
// forever with backoff on failure (the AP may be down at boot), and runs
// DHCP for as long as the association holds. It returns only when stop
// is closed.
func runAssociationLoop(deps Deps, ifi Interface, creds Credentials, stop <-chan struct{}) {
	backoff := deps.NewBackoff()
	for {
		select {
		case <-stop:
			return
		default:
		}

		if err := associate(deps, ifi, creds); err != nil {
			delay := backoff.Next()
			deps.Log("associating %s with %q failed: %v; retrying in %s", ifi.Name, creds.SSID, err, delay)
			select {
			case <-stop:
				return
			case <-deps.Clock.After(delay):
				continue
			}
		}
		backoff.Reset()
		deps.Log("%s associated with %q", ifi.Name, creds.SSID)

		runUntilDisconnect(deps, ifi, stop)
	}
}

// associate issues the nl80211 connect for creds: a plain Connect for an
// open network, or ConnectPSK with the already-resolved 256-bit PMK for
// WPA2-PSK — resolved once by CredentialSource.Credentials (either via
// PBKDF2 from a passphrase or decoded directly from a pre-hashed hex
// value), so this call site never needs to know or care which form the
// credential started as.
//
// This is unconditional on creds.Hidden: there is no prior scan step to
// gate on here at all (for a hidden or a broadcasting network alike), so a
// hidden SSID needs no separate directed-scan path — CONNECT already
// carries the target SSID straight to the driver/firmware, which performs
// its own active/directed probe for that exact SSID as part of joining.
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
// stop is closed, then returns so runAssociationLoop can reconnect.
func runUntilDisconnect(deps Deps, ifi Interface, stop <-chan struct{}) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	watchDone := make(chan struct{})
	go func() {
		defer close(watchDone)
		watchAssociation(deps, ifi, cancel, stop)
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
}

// associationPollPeriod is how often watchAssociation checks whether ifi
// is still associated. mdlayher/wifi exposes no deauth/disconnect event
// stream (only request/response nl80211 commands), so polling BSS status
// is the only portable way to detect a lost association.
const associationPollPeriod = 3 * time.Second

// watchAssociation polls ifi's association state every
// associationPollPeriod and calls disconnect (which cancels the DHCP
// context in runUntilDisconnect) as soon as it's lost, or when stop
// closes — either way, disconnect must always be called exactly once
// before this returns, or runUntilDisconnect would block forever waiting
// on the now-uncancellable DHCP context.
func watchAssociation(deps Deps, ifi Interface, disconnect context.CancelFunc, stop <-chan struct{}) {
	for {
		select {
		case <-stop:
			disconnect()
			return
		case <-deps.Clock.After(associationPollPeriod):
		}

		ok, err := deps.Wifi.Associated(ifi)
		if err != nil {
			deps.Log("checking association on %s failed: %v", ifi.Name, err)
			continue
		}
		if !ok {
			deps.Log("%s lost its WiFi association; reconnecting", ifi.Name)
			if err := deps.ClearNetworkUp(); err != nil {
				deps.Log("clearing network-up marker for %s failed: %v", ifi.Name, err)
			}
			disconnect()
			return
		}
	}
}
