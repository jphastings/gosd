package wifiup

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jphastings/gosd/cmd/gosd-init/internal/netup"
)

// This file is the wiring-level counterpart to netup's own upset_test.go:
// it proves that netup.Run (Ethernet) and wifiup.Run (WiFi), wired to one
// shared *netup.UpSet exactly the way main.go wires them, correctly
// refcount the real /run/gosd/network-up marker file across both packages
// — the pi-3b dual-interface scenario the bean (gosd-akk4) exists for.

// wiringLinks is a minimal netup.Links fake, local to this file: unlike
// wifiup's own fakeLinks (whose Watch is deliberately never used, since
// wifiup itself never calls it), this one drives netup.Run's link-event
// loop for the simulated eth0 side of the wiring test.
type wiringLinks struct {
	mu     sync.Mutex
	setUp  []string
	events chan netup.LinkEvent
}

func newWiringLinks() *wiringLinks {
	return &wiringLinks{events: make(chan netup.LinkEvent, 4)}
}

func (l *wiringLinks) SetUp(name string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.setUp = append(l.setUp, name)
	return nil
}

func (l *wiringLinks) AddAddr(string, net.IPNet) error { return nil }

func (l *wiringLinks) ReplaceDefaultRoute(string, net.IP) error { return nil }

func (l *wiringLinks) Watch(<-chan struct{}) (<-chan netup.LinkEvent, error) {
	return l.events, nil
}

// wiringDHCP always hands out the same lease, immediately, on every
// Request/Renew call — this test only cares about link/association state
// driving MarkNetworkUp/ClearNetworkUp, not DHCP's own retry/renewal
// machinery (that's netup's dhcp_test.go's job).
type wiringDHCP struct {
	lease *netup.Lease
}

func (d *wiringDHCP) Request(context.Context, string) (*netup.Lease, error) {
	return d.lease, nil
}

func (d *wiringDHCP) Renew(_ context.Context, _ string, lease *netup.Lease) (*netup.Lease, error) {
	return lease, nil
}

// TestSharedUpSetAcrossNetupAndWifiup drives the bean's exact scenario on a
// simulated pi-3b: eth0 (netup) and wlan0 (wifiup) both come up, eth0 is
// unplugged, then wlan0 loses association, then eth0 comes back — checking
// the shared marker file at every step.
func TestSharedUpSetAcrossNetupAndWifiup(t *testing.T) {
	markerPath := filepath.Join(t.TempDir(), "network-up")
	upSet := netup.NewUpSet(
		func() error { return netup.MarkNetworkUp(markerPath) },
		func() error { return netup.ClearNetworkUp(markerPath) },
	)
	markerExists := func() bool {
		_, err := os.Stat(markerPath)
		return err == nil
	}

	// --- eth0, via netup.Run ---
	ethLog := &testLog{}
	ethLinks := newWiringLinks()
	ethLease := &netup.Lease{
		Address:     net.IPNet{IP: net.IPv4(10, 0, 0, 5), Mask: net.CIDRMask(24, 32)},
		Gateway:     net.IPv4(10, 0, 0, 1),
		ObtainedAt:  time.Now(),
		RenewAfter:  time.Hour,
		RebindAfter: 2 * time.Hour,
		ExpireAfter: 3 * time.Hour,
	}
	ethDeps := netup.Deps{
		Links:           ethLinks,
		DHCP:            &wiringDHCP{lease: ethLease},
		Clock:           netup.NewRealClock(),
		NewBackoff:      func() *netup.Backoff { return netup.NewBackoff(netup.DefaultBackoffBase, netup.DefaultBackoffCap) },
		WriteResolvConf: func([]net.IP) error { return nil },
		MarkNetworkUp:   upSet.Up,
		ClearNetworkUp:  upSet.Down,
		Log:             ethLog.Printf,
	}
	ethStop := make(chan struct{})
	defer close(ethStop)
	go netup.Run(ethDeps, netup.Options{Stop: ethStop})

	ethLinks.events <- netup.LinkEvent{Name: "eth0", Up: true}

	deadline := time.Now().Add(2 * time.Second)
	for !markerExists() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !markerExists() {
		t.Fatal("marker was never created after eth0 came up")
	}

	// --- wlan0, via wifiup.Run ---
	wifiLog := &testLog{}
	wifi := &fakeWifiClient{
		interfacesResults: [][]Interface{{{Name: "wlan0", Index: 1}}},
		associatedResults: []bool{true, false},
		// The first Connect (initial association) succeeds; every
		// later one (wifiup's automatic reconnect after the
		// association loss below) fails, so wlan0 stays down rather
		// than racily re-marking the network up before the test can
		// observe the cleared marker.
		connectErrs: []error{nil, errBoom},
	}
	wifiLinks := newFakeLinks()
	wifiClock := newFakeClock(time.Unix(0, 0))
	wlanLease := &netup.Lease{
		Address:     net.IPNet{IP: net.IPv4(192, 168, 1, 9), Mask: net.CIDRMask(24, 32)},
		ObtainedAt:  wifiClock.Now(),
		RenewAfter:  time.Hour,
		RebindAfter: 2 * time.Hour,
		ExpireAfter: 3 * time.Hour,
	}
	wifiDHCP := &fakeDHCP{requestResults: []requestResult{{lease: wlanLease}}}
	creds := Credentials{SSID: "home-net", Open: true}
	wifiDeps := Deps{
		Wifi:            wifi,
		Credentials:     fakeCredentials{creds: creds, ok: true},
		Links:           wifiLinks,
		DHCP:            wifiDHCP,
		Clock:           wifiClock,
		NewBackoff:      backoffFactory(time.Second, 10*time.Second),
		WriteResolvConf: func([]net.IP) error { return nil },
		MarkNetworkUp:   upSet.Up,
		ClearNetworkUp:  upSet.Down,
		Log:             wifiLog.Printf,
	}
	wifiStop := make(chan struct{})
	defer close(wifiStop)
	go Run(wifiDeps, Options{Stop: wifiStop})

	deadline = time.Now().Add(2 * time.Second)
	for wifiDHCP.requestCallCount() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if wifiDHCP.requestCallCount() == 0 {
		t.Fatal("wlan0 never completed DHCP")
	}
	if !markerExists() {
		t.Fatal("marker unexpectedly disappeared once wlan0 joined the up set")
	}

	// eth0 goes down: wlan0 still holds the marker, so it must stay.
	ethLinks.events <- netup.LinkEvent{Name: "eth0", Up: false}

	deadline = time.Now().Add(2 * time.Second)
	for !ethLog.contains("went down") && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !ethLog.contains("went down") {
		t.Fatal("eth0 link-down was never processed")
	}
	// Give a wrong implementation a chance to clear the marker anyway.
	time.Sleep(20 * time.Millisecond)
	if !markerExists() {
		t.Error("marker was removed after eth0 alone went down, but wlan0 is still up")
	}

	// wlan0 also loses its association: now the set is empty and the
	// marker must finally be removed.
	if !advanceUntil(wifiClock, associationPollPeriod, func() bool { return wifi.associatedCallCount() >= 2 }) {
		t.Fatalf("associatedCalls = %d, want >= 2", wifi.associatedCallCount())
	}

	deadline = time.Now().Add(2 * time.Second)
	for markerExists() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if markerExists() {
		t.Error("marker still present after both eth0 and wlan0 went down")
	}

	// eth0 comes back: the marker must be recreated, not assumed stale.
	ethLinks.events <- netup.LinkEvent{Name: "eth0", Up: true}

	deadline = time.Now().Add(2 * time.Second)
	for !markerExists() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !markerExists() {
		t.Error("marker was not recreated after eth0 came back up")
	}
}
