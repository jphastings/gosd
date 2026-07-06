package wifiup

import (
	"net"
	"testing"
	"time"

	"github.com/jphastings/gosd/cmd/gosd-init/internal/netup"
)

func newTestDeps(clock *fakeClock, wifi *fakeWifiClient, links *fakeLinks, dhcp *fakeDHCP, creds CredentialSource, log *testLog) (Deps, *counter, *counter) {
	marked := &counter{}
	cleared := &counter{}
	deps := Deps{
		Wifi:        wifi,
		Credentials: creds,
		Links:       links,
		DHCP:        dhcp,
		Clock:       clock,
		NewBackoff:  backoffFactory(time.Second, 10*time.Second),
		WriteResolvConf: func([]net.IP) error {
			return nil
		},
		MarkNetworkUp: func() error {
			marked.inc()
			return nil
		},
		ClearNetworkUp: func() error {
			cleared.inc()
			return nil
		},
		Log: log.Printf,
	}
	return deps, marked, cleared
}

func TestRunSkipsEverythingWhenNoCredentialsConfigured(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	wifi := &fakeWifiClient{}
	links := newFakeLinks()
	dhcp := &fakeDHCP{}
	log := &testLog{}
	deps, _, _ := newTestDeps(clock, wifi, links, dhcp, fakeCredentials{ok: false}, log)

	Run(deps, Options{})

	if wifi.interfacesCalls != 0 {
		t.Errorf("Interfaces() called %d times, want 0 (no credentials configured)", wifi.interfacesCalls)
	}
	if !log.contains("no WiFi credentials configured") {
		t.Errorf("log missing skip message: %v", log.snapshot())
	}
}

func TestRunLogsAndSkipsOnCredentialError(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	wifi := &fakeWifiClient{}
	links := newFakeLinks()
	dhcp := &fakeDHCP{}
	log := &testLog{}
	deps, _, _ := newTestDeps(clock, wifi, links, dhcp, fakeCredentials{err: errBoom}, log)

	Run(deps, Options{})

	if wifi.interfacesCalls != 0 {
		t.Errorf("Interfaces() called %d times, want 0 (credential error)", wifi.interfacesCalls)
	}
	if !log.contains("reading WiFi credentials failed") {
		t.Errorf("log missing credential-error message: %v", log.snapshot())
	}
}

func TestRunSkipsUnsupportedSecurity(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	wifi := &fakeWifiClient{}
	links := newFakeLinks()
	dhcp := &fakeDHCP{}
	log := &testLog{}
	creds := Credentials{SSID: "enterprise-net", Unsupported: "802.1X/EAP"}
	deps, _, _ := newTestDeps(clock, wifi, links, dhcp, fakeCredentials{creds: creds, ok: true}, log)

	Run(deps, Options{})

	if wifi.interfacesCalls != 0 {
		t.Errorf("Interfaces() called %d times, want 0 (unsupported security)", wifi.interfacesCalls)
	}
	if !log.contains("802.1X/EAP") {
		t.Errorf("log missing unsupported-security message: %v", log.snapshot())
	}
}

func TestRunWaitsForInterfaceThenConnectsOpenNetwork(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	wifi := &fakeWifiClient{
		interfacesResults: [][]Interface{
			nil, // not present yet at first poll
			{{Name: "wlan0", Index: 3}},
		},
	}
	links := newFakeLinks()
	lease := &netup.Lease{
		Address:     net.IPNet{IP: net.IPv4(192, 168, 1, 9), Mask: net.CIDRMask(24, 32)},
		ObtainedAt:  clock.Now(),
		RenewAfter:  time.Hour,
		RebindAfter: 2 * time.Hour,
		ExpireAfter: 3 * time.Hour,
	}
	dhcp := &fakeDHCP{requestResults: []requestResult{{lease: lease}}}
	log := &testLog{}
	creds := Credentials{SSID: "open-net", Open: true}
	deps, marked, _ := newTestDeps(clock, wifi, links, dhcp, fakeCredentials{creds: creds, ok: true}, log)

	stop := make(chan struct{})
	defer close(stop)
	go Run(deps, Options{Stop: stop})

	if !waitAndAdvancePast(clock, 10*time.Second) {
		t.Fatal("wifiup never registered the interface-wait backoff timer")
	}

	deadline := time.Now().Add(2 * time.Second)
	for marked.load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}

	if wifi.connectCallCount() != 1 {
		t.Fatalf("Connect called %d times, want 1", wifi.connectCallCount())
	}
	if !links.sawSetUp("wlan0") {
		t.Error("wlan0 was never brought up")
	}
	if marked.load() == 0 {
		t.Error("network-up marker was never created after DHCP succeeded")
	}
	if addr, ok := links.addrFor("wlan0"); !ok || !addr.IP.Equal(lease.Address.IP) {
		t.Errorf("wlan0 address = %v, ok=%v, want %v", addr, ok, lease.Address.IP)
	}
	if dhcp.requestCallCount() != 1 {
		t.Errorf("DHCP Request called %d times, want 1", dhcp.requestCallCount())
	}
	if wifi.disconnectCalls == 0 {
		t.Error("Disconnect was never called before connecting (should clear stale state defensively)")
	}
}

func TestRunConnectsWPAPSKWithResolvedPSK(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	wifi := &fakeWifiClient{interfacesResults: [][]Interface{{{Name: "wlan0", Index: 1}}}}
	links := newFakeLinks()
	dhcp := &fakeDHCP{requestResults: []requestResult{{err: errBoom}}} // never need a real lease for this test
	log := &testLog{}
	psk, err := DerivePSK("hunter2hunter2", "home-net")
	if err != nil {
		t.Fatalf("DerivePSK() error = %v", err)
	}
	creds := Credentials{SSID: "home-net", PSK: psk}
	deps, _, _ := newTestDeps(clock, wifi, links, dhcp, fakeCredentials{creds: creds, ok: true}, log)

	stop := make(chan struct{})
	defer close(stop)
	go Run(deps, Options{Stop: stop})

	deadline := time.Now().Add(2 * time.Second)
	for wifi.connectPSKCallCount() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}

	call, ok := wifi.lastConnectPSK()
	if !ok {
		t.Fatal("ConnectPSK was never called")
	}
	if call.ssid != "home-net" || call.psk != psk {
		t.Errorf("ConnectPSK(_, %q, %x), want (_, %q, %x)", call.ssid, call.psk, "home-net", psk)
	}
}

func TestRunRetriesAssociationWithBackoffOnFailure(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	wifi := &fakeWifiClient{
		interfacesResults: [][]Interface{{{Name: "wlan0", Index: 1}}},
		connectErr:        errBoom,
	}
	links := newFakeLinks()
	dhcp := &fakeDHCP{}
	log := &testLog{}
	creds := Credentials{SSID: "flaky-net", Open: true}
	deps, _, _ := newTestDeps(clock, wifi, links, dhcp, fakeCredentials{creds: creds, ok: true}, log)

	stop := make(chan struct{})
	defer close(stop)
	go Run(deps, Options{Stop: stop})

	deadline := time.Now().Add(2 * time.Second)
	for wifi.connectCallCount() < 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if wifi.connectCallCount() < 1 {
		t.Fatal("Connect was never attempted")
	}

	if !waitAndAdvancePast(clock, 10*time.Second) {
		t.Fatal("wifiup never registered the association-retry backoff timer")
	}

	deadline = time.Now().Add(2 * time.Second)
	for wifi.connectCallCount() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if wifi.connectCallCount() < 2 {
		t.Errorf("Connect called %d times, want at least 2 (retried after failure)", wifi.connectCallCount())
	}
	if !log.contains("retrying in") {
		t.Errorf("log missing retry message: %v", log.snapshot())
	}
}

func TestRunReconnectsAfterAssociationIsLost(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	wifi := &fakeWifiClient{
		interfacesResults: [][]Interface{{{Name: "wlan0", Index: 1}}},
		associatedResults: []bool{true, false},
	}
	links := newFakeLinks()
	lease := &netup.Lease{
		Address:     net.IPNet{IP: net.IPv4(10, 1, 1, 2), Mask: net.CIDRMask(24, 32)},
		ObtainedAt:  clock.Now(),
		RenewAfter:  time.Hour,
		RebindAfter: 2 * time.Hour,
		ExpireAfter: 3 * time.Hour,
	}
	dhcp := &fakeDHCP{requestResults: []requestResult{{lease: lease}}}
	log := &testLog{}
	creds := Credentials{SSID: "home-net", Open: true}
	deps, marked, cleared := newTestDeps(clock, wifi, links, dhcp, fakeCredentials{creds: creds, ok: true}, log)

	stop := make(chan struct{})
	defer close(stop)
	go Run(deps, Options{Stop: stop})

	deadline := time.Now().Add(2 * time.Second)
	for marked.load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if marked.load() == 0 {
		t.Fatal("network was never marked up before testing the disconnect")
	}

	// A DHCP lease-renewal timer (RenewAfter: time.Hour) is also pending
	// concurrently at this point, so advance in small steps rather than
	// waiting for "a" pending timer and jumping straight to
	// associationPollPeriod — that could just as easily be racing against
	// the renewal timer's registration. First association-poll tick
	// reports still-associated (true); second reports lost (false) and
	// should trigger a reconnect.
	if !advanceUntil(clock, associationPollPeriod, func() bool { return wifi.associatedCallCount() >= 2 }) {
		t.Fatalf("associatedCalls = %d, want >= 2", wifi.associatedCallCount())
	}

	deadline = time.Now().Add(2 * time.Second)
	for cleared.load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if cleared.load() == 0 {
		t.Error("network-up marker was never cleared after the association was lost")
	}
	if !log.contains("lost its WiFi association") {
		t.Errorf("log missing disconnect message: %v", log.snapshot())
	}

	deadline = time.Now().Add(2 * time.Second)
	for wifi.connectCallCount() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if wifi.connectCallCount() < 2 {
		t.Errorf("Connect called %d times, want at least 2 (reconnected after disconnect)", wifi.connectCallCount())
	}
}

func TestRunLogsProbingMessageForHiddenNetwork(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	wifi := &fakeWifiClient{interfacesResults: [][]Interface{{{Name: "wlan0", Index: 1}}}}
	links := newFakeLinks()
	dhcp := &fakeDHCP{requestResults: []requestResult{{err: errBoom}}}
	log := &testLog{}
	creds := Credentials{SSID: "shy-net", Open: true, Hidden: true}
	deps, _, _ := newTestDeps(clock, wifi, links, dhcp, fakeCredentials{creds: creds, ok: true}, log)

	stop := make(chan struct{})
	defer close(stop)
	go Run(deps, Options{Stop: stop})

	deadline := time.Now().Add(2 * time.Second)
	for wifi.connectCallCount() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}

	if wifi.connectCallCount() == 0 {
		t.Fatal("Connect was never attempted for the hidden network")
	}
	if call := wifi.connectCalls; len(call) == 0 || call[0] != "shy-net" {
		t.Errorf("Connect calls = %v, want a directed connect to %q (no scan match required)", call, "shy-net")
	}
	if !log.contains(`hidden SSID "shy-net": probing directly; this can take longer`) {
		t.Errorf("log missing hidden-network probing message: %v", log.snapshot())
	}
}

func TestRunDoesNotLogProbingMessageForVisibleNetwork(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	wifi := &fakeWifiClient{interfacesResults: [][]Interface{{{Name: "wlan0", Index: 1}}}}
	links := newFakeLinks()
	dhcp := &fakeDHCP{requestResults: []requestResult{{err: errBoom}}}
	log := &testLog{}
	creds := Credentials{SSID: "open-net", Open: true}
	deps, _, _ := newTestDeps(clock, wifi, links, dhcp, fakeCredentials{creds: creds, ok: true}, log)

	stop := make(chan struct{})
	defer close(stop)
	go Run(deps, Options{Stop: stop})

	deadline := time.Now().Add(2 * time.Second)
	for wifi.connectCallCount() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}

	if wifi.connectCallCount() == 0 {
		t.Fatal("Connect was never attempted")
	}
	if log.contains("probing directly") {
		t.Errorf("log unexpectedly mentions hidden-network probing for a non-hidden network: %v", log.snapshot())
	}
}

func TestPickInterfacePrefersWlanPrefix(t *testing.T) {
	ifis := []Interface{{Name: "p2p0", Index: 1}, {Name: "wlan0", Index: 2}}
	got, ok := pickInterface(ifis)
	if !ok || got.Name != "wlan0" {
		t.Errorf("pickInterface(%v) = %+v, ok=%v, want wlan0", ifis, got, ok)
	}
}

func TestPickInterfaceFallsBackToFirstWhenNoWlanPrefix(t *testing.T) {
	ifis := []Interface{{Name: "moon0", Index: 1}}
	got, ok := pickInterface(ifis)
	if !ok || got.Name != "moon0" {
		t.Errorf("pickInterface(%v) = %+v, ok=%v, want moon0", ifis, got, ok)
	}
}

func TestPickInterfaceReportsNotFoundWhenEmpty(t *testing.T) {
	if _, ok := pickInterface(nil); ok {
		t.Error("pickInterface(nil) ok = true, want false")
	}
}
