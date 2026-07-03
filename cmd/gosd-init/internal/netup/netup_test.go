package netup

import (
	"net"
	"testing"
	"time"
)

func TestIsWiredInterfaceMatchesLockedPatterns(t *testing.T) {
	cases := map[string]bool{
		"eth0":   true,
		"eth1":   true,
		"end0":   true,
		"enp1s0": true,
		"lo":     false,
		"wlan0":  false,
		"":       false,
		"e":      false,
	}
	for name, want := range cases {
		if got := isWiredInterface(name); got != want {
			t.Errorf("isWiredInterface(%q) = %v, want %v", name, got, want)
		}
	}
}

func newTestRunDeps(clock *fakeClock, links *fakeLinks, dhcp *fakeDHCP, log *testLog) (Deps, *counter, *counter) {
	marked := &counter{}
	cleared := &counter{}
	deps := Deps{
		Links:      links,
		DHCP:       dhcp,
		Clock:      clock,
		NewBackoff: func() *Backoff { return noJitterBackoff(time.Second, 10*time.Second) },
		WriteResolvConf: func(dns []net.IP) error {
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

func TestRunBringsLoUpAndConfiguresLeaseOnLinkUp(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	links := newFakeLinks()
	lease := &Lease{
		Address:     net.IPNet{IP: net.IPv4(192, 168, 1, 5), Mask: net.CIDRMask(24, 32)},
		Gateway:     net.IPv4(192, 168, 1, 1),
		DNS:         []net.IP{net.IPv4(8, 8, 8, 8)},
		ObtainedAt:  clock.Now(),
		RenewAfter:  time.Hour,
		RebindAfter: 2 * time.Hour,
		ExpireAfter: 3 * time.Hour,
	}
	dhcp := &fakeDHCP{requestResults: []requestResult{{lease: lease}}}
	log := &testLog{}
	deps, marked, _ := newTestRunDeps(clock, links, dhcp, log)

	stop := make(chan struct{})
	go Run(deps, Options{Stop: stop})

	deadline := time.Now().Add(2 * time.Second)
	for !links.sawSetUp("lo") && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !links.sawSetUp("lo") {
		t.Fatal("lo was never brought up")
	}

	links.events <- LinkEvent{Name: "eth0", Up: true}

	deadline = time.Now().Add(2 * time.Second)
	for marked.load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}

	if !links.sawSetUp("eth0") {
		t.Error("eth0 was never brought up")
	}
	addr, ok := links.addrFor("eth0")
	if !ok || addr.IP.String() != "192.168.1.5" {
		t.Errorf("eth0 address = %v, ok=%v, want 192.168.1.5", addr, ok)
	}
	gw, ok := links.routeFor("eth0")
	if !ok || !gw.Equal(net.IPv4(192, 168, 1, 1)) {
		t.Errorf("eth0 default route = %v, ok=%v, want 192.168.1.1", gw, ok)
	}
	if marked.load() == 0 {
		t.Error("network-up marker was never created")
	}

	close(stop)
}

func TestRunIgnoresNonWiredInterfaces(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	links := newFakeLinks()
	dhcp := &fakeDHCP{}
	log := &testLog{}
	deps, _, _ := newTestRunDeps(clock, links, dhcp, log)

	stop := make(chan struct{})
	go Run(deps, Options{Stop: stop})

	deadline := time.Now().Add(2 * time.Second)
	for !links.sawSetUp("lo") && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}

	links.events <- LinkEvent{Name: "wlan0", Up: true}
	time.Sleep(20 * time.Millisecond) // give a wrong implementation a chance to react

	if dhcp.requestCallCount() != 0 {
		t.Errorf("DHCP was attempted on a non-wired interface: %d Request calls", dhcp.requestCallCount())
	}

	close(stop)
}

func TestRunHandlesLinkFlapByStoppingAndClearingMarker(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	links := newFakeLinks()
	lease := &Lease{
		Address:     net.IPNet{IP: net.IPv4(10, 0, 0, 2), Mask: net.CIDRMask(24, 32)},
		ObtainedAt:  clock.Now(),
		RenewAfter:  time.Hour,
		RebindAfter: 2 * time.Hour,
		ExpireAfter: 3 * time.Hour,
	}
	dhcp := &fakeDHCP{requestResults: []requestResult{{lease: lease}}}
	log := &testLog{}
	deps, marked, cleared := newTestRunDeps(clock, links, dhcp, log)

	stop := make(chan struct{})
	go Run(deps, Options{Stop: stop})

	links.events <- LinkEvent{Name: "eth0", Up: true}
	deadline := time.Now().Add(2 * time.Second)
	for marked.load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if marked.load() == 0 {
		t.Fatal("network was never marked up before testing the flap")
	}

	links.events <- LinkEvent{Name: "eth0", Up: false}

	deadline = time.Now().Add(2 * time.Second)
	for cleared.load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if cleared.load() == 0 {
		t.Error("network-up marker was never cleared after link down")
	}
	if !log.contains("went down") {
		t.Errorf("log missing link-down message: %v", log.snapshot())
	}

	// Replug: DHCP should run again on the same interface.
	links.events <- LinkEvent{Name: "eth0", Up: true}
	deadline = time.Now().Add(2 * time.Second)
	for dhcp.requestCallCount() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if dhcp.requestCallCount() < 2 {
		t.Errorf("DHCP did not restart after replug: Request called %d times", dhcp.requestCallCount())
	}

	close(stop)
}
