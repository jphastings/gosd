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
		MarkNetworkUp: func(_ string) error {
			marked.inc()
			return nil
		},
		ClearNetworkUp: func(_ string) error {
			cleared.inc()
			return nil
		},
		Log: log.Printf,
	}
	return deps, marked, cleared
}

// waitForLog polls until log contains substr or a generous deadline passes,
// used by the three tests below: with decision 5's restructure Run no
// longer returns immediately when there's nothing for the association loop
// to do (the watcher, if any, still runs), so these can no longer just call
// Run synchronously and check its log afterward.
func waitForLog(t *testing.T, log *testLog, substr string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !log.contains(substr) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !log.contains(substr) {
		t.Fatalf("log missing %q: %v", substr, log.snapshot())
	}
}

func TestRunSkipsEverythingWhenNoCredentialsConfigured(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	wifi := &fakeWifiClient{}
	links := newFakeLinks()
	dhcp := &fakeDHCP{}
	log := &testLog{}
	deps, _, _ := newTestDeps(clock, wifi, links, dhcp, fakeCredentials{ok: false}, log)

	stop := make(chan struct{})
	defer close(stop)
	go Run(deps, Options{Stop: stop})

	waitForLog(t, log, "no WiFi credentials configured")
	if wifi.interfacesCalls != 0 {
		t.Errorf("Interfaces() called %d times, want 0 (no credentials configured)", wifi.interfacesCalls)
	}
}

func TestRunLogsAndSkipsOnCredentialError(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	wifi := &fakeWifiClient{}
	links := newFakeLinks()
	dhcp := &fakeDHCP{}
	log := &testLog{}
	deps, _, _ := newTestDeps(clock, wifi, links, dhcp, fakeCredentials{err: errBoom}, log)

	stop := make(chan struct{})
	defer close(stop)
	go Run(deps, Options{Stop: stop})

	waitForLog(t, log, "reading WiFi credentials failed")
	if wifi.interfacesCalls != 0 {
		t.Errorf("Interfaces() called %d times, want 0 (credential error)", wifi.interfacesCalls)
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

	stop := make(chan struct{})
	defer close(stop)
	go Run(deps, Options{Stop: stop})

	waitForLog(t, log, "802.1X/EAP")
	if wifi.interfacesCalls != 0 {
		t.Errorf("Interfaces() called %d times, want 0 (unsupported security)", wifi.interfacesCalls)
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

func TestRunSkipsInterfaceWithoutHandshakeOffload(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	// wlan0 is a phantom (no 4-way-handshake offload, like
	// mac80211_hwsim's simulated radios); wlan1 is the real radio.
	wifi := &fakeWifiClient{
		interfacesResults:  [][]Interface{{{Name: "wlan0", Index: 1}, {Name: "wlan1", Index: 2}}},
		offloadUnsupported: map[string]bool{"wlan0": true},
	}
	links := newFakeLinks()
	dhcp := &fakeDHCP{requestResults: []requestResult{{err: errBoom}}}
	log := &testLog{}
	creds := Credentials{SSID: "home-net", PSK: [32]byte{1}}
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
	if call.ifname != "wlan1" {
		t.Errorf("ConnectPSK used %s, want wlan1 (the offload-capable candidate)", call.ifname)
	}
	if !links.sawSetUp("wlan1") {
		t.Error("wlan1 was never brought up")
	}
	if !log.contains("wlan0 cannot do firmware-offloaded WPA2-PSK (missing 4WAY_HANDSHAKE_STA_PSK)") {
		t.Errorf("log missing actionable offload error for wlan0: %v", log.snapshot())
	}
	if !log.contains("mac80211_hwsim") && !log.contains("gosd-6nl2") {
		t.Errorf("offload error is missing its phantom-radio pointer: %v", log.snapshot())
	}
}

func TestRunProceedsWithSoleInterfaceLackingOffload(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	wifi := &fakeWifiClient{
		interfacesResults:  [][]Interface{{{Name: "wlan0", Index: 1}}},
		offloadUnsupported: map[string]bool{"wlan0": true},
	}
	links := newFakeLinks()
	dhcp := &fakeDHCP{requestResults: []requestResult{{err: errBoom}}}
	log := &testLog{}
	creds := Credentials{SSID: "home-net", PSK: [32]byte{1}}
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
		t.Fatal("ConnectPSK was never attempted on the sole interface")
	}
	if call.ifname != "wlan0" {
		t.Errorf("ConnectPSK used %s, want wlan0 (no capable candidate exists, so proceed honestly)", call.ifname)
	}
	if !log.contains("wlan0 cannot do firmware-offloaded WPA2-PSK") {
		t.Errorf("log missing actionable offload error: %v", log.snapshot())
	}
}

func TestRunProceedsWhenOffloadCheckFails(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	wifi := &fakeWifiClient{
		interfacesResults: [][]Interface{{{Name: "wlan0", Index: 1}}},
		offloadErr:        errBoom,
	}
	links := newFakeLinks()
	dhcp := &fakeDHCP{requestResults: []requestResult{{err: errBoom}}}
	log := &testLog{}
	creds := Credentials{SSID: "home-net", PSK: [32]byte{1}}
	deps, _, _ := newTestDeps(clock, wifi, links, dhcp, fakeCredentials{creds: creds, ok: true}, log)

	stop := make(chan struct{})
	defer close(stop)
	go Run(deps, Options{Stop: stop})

	deadline := time.Now().Add(2 * time.Second)
	for wifi.connectPSKCallCount() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}

	if wifi.connectPSKCallCount() == 0 {
		t.Fatal("ConnectPSK was never attempted (a failed check must not skip a possibly-real radio)")
	}
	if !log.contains("checking WPA2 handshake offload on wlan0 failed") {
		t.Errorf("log missing check-failure notice: %v", log.snapshot())
	}
}

func TestRunSkipsOffloadCheckForOpenNetwork(t *testing.T) {
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
	if n := wifi.offloadCheckCount(); n != 0 {
		t.Errorf("SupportsOffloadedHandshake called %d times for an open network, want 0 (open joins carry no PMK)", n)
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

// TestRunNeverAssociatingBacksOffAcrossRepeatedCycles guards gosd-vcnr: a
// wrong PSK gets its CONNECT acked by the kernel every time (the ack only
// means the request was accepted, not that the 4-way handshake
// completed), so association never confirms. Before the fix, the backoff
// was reset on that ack alone, and the loop's "lost association" path
// never consulted it at all — retrying at a fixed ~3s cadence (the
// association poll period) forever. This checks, across two such cycles,
// that a retry always waits for a backoff timer on top of the poll,
// never firing in the brief real-time window right after the poll alone
// elapses.
func TestRunNeverAssociatingBacksOffAcrossRepeatedCycles(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	wifi := &fakeWifiClient{
		interfacesResults: [][]Interface{{{Name: "wlan0", Index: 1}}},
		// CONNECT is always acked, but Associated never reports true.
		associatedResults: []bool{false},
	}
	links := newFakeLinks()
	dhcp := &fakeDHCP{requestResults: []requestResult{{err: errBoom}}} // never need a real lease
	log := &testLog{}
	creds := Credentials{SSID: "wrong-psk-net", Open: true}
	deps, _, _ := newTestDeps(clock, wifi, links, dhcp, fakeCredentials{creds: creds, ok: true}, log)

	stop := make(chan struct{})
	defer close(stop)
	go Run(deps, Options{Stop: stop})

	for cycle := 1; cycle <= 2; cycle++ {
		deadline := time.Now().Add(2 * time.Second)
		for wifi.connectCallCount() < cycle && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		if wifi.connectCallCount() < cycle {
			t.Fatalf("cycle %d: Connect was never attempted (calls=%d)", cycle, wifi.connectCallCount())
		}

		if !advanceUntil(clock, associationPollPeriod, func() bool { return wifi.associatedCallCount() >= cycle }) {
			t.Fatalf("cycle %d: associatedCalls = %d, want >= %d", cycle, wifi.associatedCallCount(), cycle)
		}

		// Give any immediate (real-time, not fake-clock-gated) retry —
		// exactly what the pre-fix bug did — a generous window to
		// happen before asserting it hasn't.
		time.Sleep(20 * time.Millisecond)
		if wifi.connectCallCount() != cycle {
			t.Fatalf("cycle %d: Connect called %d times right after the association poll fired, want %d (a backoff wait must still be pending)", cycle, wifi.connectCallCount(), cycle)
		}

		if !waitAndAdvancePast(clock, 10*time.Second) {
			t.Fatalf("cycle %d: backoff retry timer was never registered", cycle)
		}

		deadline = time.Now().Add(2 * time.Second)
		for wifi.connectCallCount() <= cycle && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		if wifi.connectCallCount() <= cycle {
			t.Fatalf("cycle %d: Connect was never retried after the backoff elapsed", cycle)
		}
	}

	if !log.contains("accepted the connect but association was never confirmed") {
		t.Errorf("log missing never-confirmed-association retry message: %v", log.snapshot())
	}
}

// TestRunDoesNotResetBackoffOnImmediateDisassociation is the minimal,
// single-cycle version of the gosd-vcnr regression: one CONNECT ack
// followed immediately by a poll that never saw association must not
// reset the backoff (equivalently: must not retry without waiting for
// one).
func TestRunDoesNotResetBackoffOnImmediateDisassociation(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	wifi := &fakeWifiClient{
		interfacesResults: [][]Interface{{{Name: "wlan0", Index: 1}}},
		associatedResults: []bool{false},
	}
	links := newFakeLinks()
	dhcp := &fakeDHCP{requestResults: []requestResult{{err: errBoom}}} // never need a real lease
	log := &testLog{}
	creds := Credentials{SSID: "wrong-psk-net", Open: true}
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
	if !log.contains("connect accepted") {
		deadline = time.Now().Add(2 * time.Second)
		for !log.contains("connect accepted") && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
	}
	if !log.contains("connect accepted") {
		t.Fatalf("log missing connect-accepted message: %v", log.snapshot())
	}

	if !advanceUntil(clock, associationPollPeriod, func() bool { return wifi.associatedCallCount() >= 1 }) {
		t.Fatalf("associatedCalls = %d, want >= 1", wifi.associatedCallCount())
	}

	// The pre-fix bug reset the backoff on the ack itself, so the retry
	// after this immediate disassociation needed no further wait at
	// all — Connect would already have been called a second time by
	// now. Give any such immediate retry a generous real-time window to
	// happen before asserting it hasn't.
	time.Sleep(20 * time.Millisecond)
	if wifi.connectCallCount() != 1 {
		t.Fatalf("Connect called %d times right after the ack-then-immediate-disassociation, want 1 (the backoff must not have been reset/skipped)", wifi.connectCallCount())
	}
	if !log.contains("accepted the connect but association was never confirmed") {
		t.Errorf("log missing never-confirmed-association message: %v", log.snapshot())
	}

	if !waitAndAdvancePast(clock, 10*time.Second) {
		t.Fatal("backoff retry timer was never registered")
	}
	deadline = time.Now().Add(2 * time.Second)
	for wifi.connectCallCount() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if wifi.connectCallCount() < 2 {
		t.Fatal("Connect was never retried after the backoff elapsed")
	}
}

// TestRunResetsBackoffAfterGenuineAssociation checks the other half of
// gosd-vcnr's fix: a connection that genuinely associates (Associated
// observed true, even briefly) before being lost must still reset the
// backoff, so a later unrelated failure doesn't inherit a stale, grown
// delay. Cycles 1-2 grow the backoff via outright connect failures;
// cycle 3 genuinely associates and is then lost; cycle 4 fails again
// immediately after. Backoff.Next's full jitter always draws strictly
// less than the pre-jitter delay, and a freshly reset backoff's first
// call uses the base delay — so advancing the clock by exactly that base
// deterministically triggers cycle 4's retry only if the reset actually
// happened; a backoff still carrying cycles 1-2's growth would need
// materially longer far more often than not.
func TestRunResetsBackoffAfterGenuineAssociation(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	wifi := &fakeWifiClient{
		interfacesResults: [][]Interface{{{Name: "wlan0", Index: 1}}},
		connectErrs:       []error{errBoom, errBoom, nil, errBoom},
		associatedResults: []bool{true, false},
	}
	links := newFakeLinks()
	dhcp := &fakeDHCP{requestResults: []requestResult{{err: errBoom}}} // never need a real lease
	log := &testLog{}
	creds := Credentials{SSID: "home-net", Open: true}
	deps, _, _ := newTestDeps(clock, wifi, links, dhcp, fakeCredentials{creds: creds, ok: true}, log)

	stop := make(chan struct{})
	defer close(stop)
	go Run(deps, Options{Stop: stop})

	// Cycles 1-2: two outright connect failures, each gated by a backoff
	// wait, growing the underlying delay past its base.
	for cycle := 1; cycle <= 2; cycle++ {
		deadline := time.Now().Add(2 * time.Second)
		for wifi.connectCallCount() < cycle && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		if wifi.connectCallCount() < cycle {
			t.Fatalf("cycle %d: Connect was never attempted", cycle)
		}
		if !waitAndAdvancePast(clock, 10*time.Second) {
			t.Fatalf("cycle %d: backoff retry timer was never registered", cycle)
		}
	}

	// Cycle 3: the third Connect call succeeds and genuinely associates,
	// then is lost.
	deadline := time.Now().Add(2 * time.Second)
	for wifi.connectCallCount() < 3 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if wifi.connectCallCount() < 3 {
		t.Fatal("cycle 3: Connect was never attempted")
	}
	if !advanceUntil(clock, associationPollPeriod, func() bool { return wifi.associatedCallCount() >= 2 }) {
		t.Fatalf("associatedCalls = %d, want >= 2", wifi.associatedCallCount())
	}

	// Cycle 4: wait for its failure to be logged (this is wifiup's own
	// "associating ... failed" message, distinct from netup's DHCP-retry
	// logging, which shares the "; retrying in" suffix but not this
	// prefix) — cycles 1, 2 and 4 each log one, cycle 3 logs none.
	retryMsg := `associating wlan0 with "home-net" failed`
	deadline = time.Now().Add(2 * time.Second)
	for log.count(retryMsg) < 3 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if n := log.count(retryMsg); n < 3 {
		t.Fatalf("cycle 4's failure was never logged (count=%d, want >= 3): %v", n, log.snapshot())
	}
	time.Sleep(10 * time.Millisecond) // let the just-logged retry register its timer
	clock.Advance(time.Second)        // == the test backoff factory's base delay

	deadline = time.Now().Add(2 * time.Second)
	for wifi.connectCallCount() < 5 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if wifi.connectCallCount() < 5 {
		t.Errorf("Connect called %d times, want 5: backoff was not reset after cycle 3's genuine association", wifi.connectCallCount())
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

// driveAssociationLoss brings wifi up to a marked (DHCP-complete)
// association, then advances the clock through association polls until
// the loss is noticed and logged. wifi must be scripted with
// associatedResults ending in false.
func driveAssociationLoss(t *testing.T, clock *fakeClock, wifi *fakeWifiClient, marked *counter, log *testLog) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for marked.load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if marked.load() == 0 {
		t.Fatal("network was never marked up before testing the disconnect")
	}

	if !advanceUntil(clock, associationPollPeriod, func() bool { return wifi.associatedCallCount() >= 2 }) {
		t.Fatalf("associatedCalls = %d, want >= 2", wifi.associatedCallCount())
	}

	deadline = time.Now().Add(2 * time.Second)
	for !log.contains("lost its WiFi association") && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !log.contains("lost its WiFi association") {
		t.Fatalf("log missing disconnect message: %v", log.snapshot())
	}
}

func newLossScriptedTest(clock *fakeClock) (*fakeWifiClient, *fakeLinks, *fakeDHCP) {
	wifi := &fakeWifiClient{
		interfacesResults: [][]Interface{{{Name: "wlan0", Index: 1}}},
		associatedResults: []bool{true, false},
	}
	lease := &netup.Lease{
		Address:     net.IPNet{IP: net.IPv4(10, 1, 1, 2), Mask: net.CIDRMask(24, 32)},
		ObtainedAt:  clock.Now(),
		RenewAfter:  time.Hour,
		RebindAfter: 2 * time.Hour,
		ExpireAfter: 3 * time.Hour,
	}
	return wifi, newFakeLinks(), &fakeDHCP{requestResults: []requestResult{{lease: lease}}}
}

// TestRunAssociationLossFlushesAddressesBeforeReconnectLease mirrors
// netup's link-down teardown for bean gosd-1lx7: losing the WiFi
// association must flush the stale lease's address (not just cancel DHCP
// and clear the marker), and a reconnect that lands a different lease
// address must end up with only the new address, not both stacked up.
func TestRunAssociationLossFlushesAddressesBeforeReconnectLease(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	wifi := &fakeWifiClient{
		interfacesResults: [][]Interface{{{Name: "wlan0", Index: 1}}},
		associatedResults: []bool{true, false},
	}
	links := newFakeLinks()
	first := &netup.Lease{
		Address:     net.IPNet{IP: net.IPv4(10, 1, 1, 2), Mask: net.CIDRMask(24, 32)},
		ObtainedAt:  clock.Now(),
		RenewAfter:  time.Hour,
		RebindAfter: 2 * time.Hour,
		ExpireAfter: 3 * time.Hour,
	}
	second := &netup.Lease{
		Address:     net.IPNet{IP: net.IPv4(10, 1, 1, 9), Mask: net.CIDRMask(24, 32)},
		ObtainedAt:  clock.Now(),
		RenewAfter:  time.Hour,
		RebindAfter: 2 * time.Hour,
		ExpireAfter: 3 * time.Hour,
	}
	dhcp := &fakeDHCP{requestResults: []requestResult{{lease: first}, {lease: second}}}
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
	if addr, ok := links.addrFor("wlan0"); !ok || !addr.IP.Equal(first.Address.IP) {
		t.Fatalf("wlan0 address after first association = %v, ok=%v, want %v", addr, ok, first.Address.IP)
	}

	if !advanceUntil(clock, associationPollPeriod, func() bool { return wifi.associatedCallCount() >= 2 }) {
		t.Fatalf("associatedCalls = %d, want >= 2", wifi.associatedCallCount())
	}
	deadline = time.Now().Add(2 * time.Second)
	for cleared.load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := links.flushCountFor("wlan0"); got == 0 {
		t.Error("wlan0 addresses were never flushed after the association was lost")
	}
	// Not asserting the address is gone here: the fake DHCP client
	// resolves synchronously, so the automatic reconnect (fired by the
	// same loss that triggered the flush) may already have landed the
	// second lease's address by the time this line runs. The flush
	// having happened at least once, plus the exactly-one-address check
	// below, together prove no stale accumulation occurred.

	deadline = time.Now().Add(2 * time.Second)
	for marked.load() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if marked.load() < 2 {
		t.Fatal("network was never marked up again after reconnecting")
	}
	addrs := links.addrsFor("wlan0")
	if len(addrs) != 1 || !addrs[0].IP.Equal(second.Address.IP) {
		t.Errorf("wlan0 addresses after reconnect = %v, want exactly [%v]", addrs, second.Address)
	}
}

func TestRunLogsDisconnectReasonWhenObserved(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	wifi, links, dhcp := newLossScriptedTest(clock)
	log := &testLog{}
	creds := Credentials{SSID: "home-net", Open: true}
	deps, marked, _ := newTestDeps(clock, wifi, links, dhcp, fakeCredentials{creds: creds, ok: true}, log)

	stop := make(chan struct{})
	defer close(stop)
	go Run(deps, Options{Stop: stop})

	deadline := time.Now().Add(2 * time.Second)
	for marked.load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	// Only now — after association — does the AP deauth, as the mlme
	// event stream would report it.
	wifi.disconnectWatcher.setReason(DisconnectReason{Code: 15, FromAP: true})

	driveAssociationLoss(t, clock, wifi, marked, log)

	want := "wlan0 lost its WiFi association (reason 15 (4-way handshake timeout), reported by AP); reconnecting"
	if !log.contains(want) {
		t.Errorf("log missing reason-annotated disconnect message %q: %v", want, log.snapshot())
	}
}

func TestRunDiscardsReasonObservedBeforeAssociation(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	wifi, links, dhcp := newLossScriptedTest(clock)
	// A reason event from before the association — e.g. from associate's
	// own defensive Disconnect — must not be pinned on a later loss.
	wifi.disconnectWatcher.setReason(DisconnectReason{Code: 3})
	log := &testLog{}
	creds := Credentials{SSID: "home-net", Open: true}
	deps, marked, _ := newTestDeps(clock, wifi, links, dhcp, fakeCredentials{creds: creds, ok: true}, log)

	stop := make(chan struct{})
	defer close(stop)
	go Run(deps, Options{Stop: stop})

	driveAssociationLoss(t, clock, wifi, marked, log)

	if !log.contains("lost its WiFi association; reconnecting") {
		t.Errorf("log missing plain (reason-free) disconnect message: %v", log.snapshot())
	}
	if log.contains("(reason") {
		t.Errorf("stale pre-association reason was attributed to the loss: %v", log.snapshot())
	}
}

func TestRunStillReconnectsWhenDisconnectWatchFails(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	wifi, links, dhcp := newLossScriptedTest(clock)
	wifi.watchErr = errBoom
	log := &testLog{}
	creds := Credentials{SSID: "home-net", Open: true}
	deps, marked, _ := newTestDeps(clock, wifi, links, dhcp, fakeCredentials{creds: creds, ok: true}, log)

	stop := make(chan struct{})
	defer close(stop)
	go Run(deps, Options{Stop: stop})

	driveAssociationLoss(t, clock, wifi, marked, log)

	if !log.contains("subscribing to wlan0 disconnect events failed") {
		t.Errorf("log missing subscription-failure notice: %v", log.snapshot())
	}
	if !log.contains("lost its WiFi association; reconnecting") {
		t.Errorf("log missing plain (reason-free) disconnect message: %v", log.snapshot())
	}

	deadline := time.Now().Add(2 * time.Second)
	for wifi.connectCallCount() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if wifi.connectCallCount() < 2 {
		t.Errorf("Connect called %d times, want at least 2 (reconnect must survive a watch failure)", wifi.connectCallCount())
	}
}

func TestRunClosesDisconnectWatcherOnStop(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	wifi := &fakeWifiClient{
		interfacesResults: [][]Interface{{{Name: "wlan0", Index: 1}}},
		connectErr:        errBoom, // park the loop in its retry select
	}
	links := newFakeLinks()
	dhcp := &fakeDHCP{}
	log := &testLog{}
	creds := Credentials{SSID: "home-net", Open: true}
	deps, _, _ := newTestDeps(clock, wifi, links, dhcp, fakeCredentials{creds: creds, ok: true}, log)

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		Run(deps, Options{Stop: stop})
		close(done)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for wifi.connectCallCount() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if wifi.connectCallCount() == 0 {
		t.Fatal("Connect was never attempted")
	}

	close(stop)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after stop closed")
	}
	if !wifi.disconnectWatcher.wasClosed() {
		t.Error("disconnect watcher was not closed when the loop ended")
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
