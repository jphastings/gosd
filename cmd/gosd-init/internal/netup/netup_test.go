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

func TestRunBringsUpAFreshlyDiscoveredDownInterface(t *testing.T) {
	// Mirrors what Watch's ListExisting actually reports for a real NIC
	// (virtio-net included) that has never been brought up: an initial
	// "down" event, with no external udev/NetworkManager ever calling `ip
	// link set up`. Without gosd-init doing it itself, this interface
	// would never come up and DHCP would never start (see the qemu-virt
	// boot bug this regresses against).
	clock := newFakeClock(time.Unix(0, 0))
	links := newFakeLinks()
	lease := &Lease{
		Address:     net.IPNet{IP: net.IPv4(192, 168, 1, 9), Mask: net.CIDRMask(24, 32)},
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

	links.events <- LinkEvent{Name: "eth0", Up: false}

	deadline := time.Now().Add(2 * time.Second)
	for !links.sawSetUp("eth0") && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !links.sawSetUp("eth0") {
		t.Fatal("a freshly discovered down interface was never brought up")
	}
	if dhcp.requestCallCount() != 0 {
		t.Error("DHCP started before the interface reported operationally up")
	}

	// The kernel reports the resulting admin-up transition as its own event.
	links.events <- LinkEvent{Name: "eth0", Up: true}

	deadline = time.Now().Add(2 * time.Second)
	for marked.load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if marked.load() == 0 {
		t.Error("DHCP never started once the interface reported operationally up")
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

// TestRunLinkDownFlushesAddresses guards bean gosd-1lx7: link-down must
// remove the addresses the last lease assigned, not just cancel DHCP and
// clear the marker, or a downed interface keeps announcing a now-dead
// address to anything that reads it (e.g. mDNS).
func TestRunLinkDownFlushesAddresses(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	links := newFakeLinks()
	lease := &Lease{
		Address:     net.IPNet{IP: net.IPv4(192, 168, 1, 50), Mask: net.CIDRMask(24, 32)},
		ObtainedAt:  clock.Now(),
		RenewAfter:  time.Hour,
		RebindAfter: 2 * time.Hour,
		ExpireAfter: 3 * time.Hour,
	}
	dhcp := &fakeDHCP{requestResults: []requestResult{{lease: lease}}}
	log := &testLog{}
	deps, marked, cleared := newTestRunDeps(clock, links, dhcp, log)

	stop := make(chan struct{})
	defer close(stop)
	go Run(deps, Options{Stop: stop})

	links.events <- LinkEvent{Name: "eth0", Up: true}
	deadline := time.Now().Add(2 * time.Second)
	for marked.load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if addr, ok := links.addrFor("eth0"); !ok || !addr.IP.Equal(lease.Address.IP) {
		t.Fatalf("eth0 address before link-down = %v, ok=%v, want %v", addr, ok, lease.Address.IP)
	}

	links.events <- LinkEvent{Name: "eth0", Up: false}
	deadline = time.Now().Add(2 * time.Second)
	for cleared.load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if _, ok := links.addrFor("eth0"); ok {
		t.Error("eth0 still has an address after link-down; FlushAddrs was not called")
	}
}

// TestRunLinkDownFlushesOnlyThatInterfacesAddresses checks the interplay
// with gosd-akk4's per-interface refcounted marker: one interface's
// link-down must flush only its own addresses, never a different,
// still-up interface's (e.g. a dual-interface board's Ethernet going down
// while WiFi stays up).
func TestRunLinkDownFlushesOnlyThatInterfacesAddresses(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	links := newFakeLinks()
	lease0 := &Lease{
		Address:     net.IPNet{IP: net.IPv4(192, 168, 1, 50), Mask: net.CIDRMask(24, 32)},
		ObtainedAt:  clock.Now(),
		RenewAfter:  time.Hour,
		RebindAfter: 2 * time.Hour,
		ExpireAfter: 3 * time.Hour,
	}
	lease1 := &Lease{
		Address:     net.IPNet{IP: net.IPv4(192, 168, 1, 60), Mask: net.CIDRMask(24, 32)},
		ObtainedAt:  clock.Now(),
		RenewAfter:  time.Hour,
		RebindAfter: 2 * time.Hour,
		ExpireAfter: 3 * time.Hour,
	}
	dhcp := &fakeDHCP{requestResults: []requestResult{{lease: lease0}, {lease: lease1}}}
	log := &testLog{}
	deps, marked, cleared := newTestRunDeps(clock, links, dhcp, log)

	stop := make(chan struct{})
	defer close(stop)
	go Run(deps, Options{Stop: stop})

	links.events <- LinkEvent{Name: "eth0", Up: true}
	deadline := time.Now().Add(2 * time.Second)
	for marked.load() < 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if marked.load() < 1 {
		t.Fatal("eth0 was never marked up")
	}

	links.events <- LinkEvent{Name: "eth1", Up: true}
	deadline = time.Now().Add(2 * time.Second)
	for marked.load() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if marked.load() < 2 {
		t.Fatal("eth1 was never marked up")
	}

	links.events <- LinkEvent{Name: "eth0", Up: false}
	deadline = time.Now().Add(2 * time.Second)
	for cleared.load() < 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if cleared.load() < 1 {
		t.Fatal("eth0 link-down was never processed")
	}

	if _, ok := links.addrFor("eth0"); ok {
		t.Error("eth0 still has an address after its own link-down")
	}
	if addr, ok := links.addrFor("eth1"); !ok || !addr.IP.Equal(lease1.Address.IP) {
		t.Errorf("eth1 address = %v, ok=%v, want %v (must survive eth0's link-down)", addr, ok, lease1.Address.IP)
	}
	if got := links.flushCountFor("eth1"); got != 0 {
		t.Errorf("eth1 FlushAddrs called %d times, want 0 (only eth0 went down)", got)
	}
}

// TestRunReplugWithNewLeaseAddressEndsWithOnlyNewAddress is the
// stale-accumulation regression test for bean gosd-1lx7: before the fix,
// AddAddr's AddrReplace only replaces an identical address/prefix, so a
// replug that got a different lease address left the interface carrying
// both the old and the new address.
func TestRunReplugWithNewLeaseAddressEndsWithOnlyNewAddress(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	links := newFakeLinks()
	first := &Lease{
		Address:     net.IPNet{IP: net.IPv4(192, 168, 1, 50), Mask: net.CIDRMask(24, 32)},
		ObtainedAt:  clock.Now(),
		RenewAfter:  time.Hour,
		RebindAfter: 2 * time.Hour,
		ExpireAfter: 3 * time.Hour,
	}
	second := &Lease{
		Address:     net.IPNet{IP: net.IPv4(192, 168, 1, 87), Mask: net.CIDRMask(24, 32)},
		ObtainedAt:  clock.Now(),
		RenewAfter:  time.Hour,
		RebindAfter: 2 * time.Hour,
		ExpireAfter: 3 * time.Hour,
	}
	dhcp := &fakeDHCP{requestResults: []requestResult{{lease: first}, {lease: second}}}
	log := &testLog{}
	deps, marked, cleared := newTestRunDeps(clock, links, dhcp, log)

	stop := make(chan struct{})
	defer close(stop)
	go Run(deps, Options{Stop: stop})

	links.events <- LinkEvent{Name: "eth0", Up: true}
	deadline := time.Now().Add(2 * time.Second)
	for marked.load() < 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if marked.load() < 1 {
		t.Fatal("eth0 was never marked up before the replug")
	}

	links.events <- LinkEvent{Name: "eth0", Up: false}
	deadline = time.Now().Add(2 * time.Second)
	for cleared.load() < 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}

	links.events <- LinkEvent{Name: "eth0", Up: true}
	deadline = time.Now().Add(2 * time.Second)
	for marked.load() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if marked.load() < 2 {
		t.Fatal("eth0 was never marked up again after replug")
	}

	addrs := links.addrsFor("eth0")
	if len(addrs) != 1 || !addrs[0].IP.Equal(second.Address.IP) {
		t.Errorf("eth0 addresses after replug = %v, want exactly [%v] (the new lease only)", addrs, second.Address)
	}
}

func TestOnLeaseForFlushesOnlyWhenTheAddressChanges(t *testing.T) {
	links := newFakeLinks()
	log := &testLog{}
	deps := Deps{
		Links:           links,
		WriteResolvConf: func([]net.IP) error { return nil },
		MarkNetworkUp:   func(string) error { return nil },
		Log:             log.Printf,
	}
	apply := onLeaseFor(deps, "eth0")

	addrA := net.IPNet{IP: net.IPv4(192, 168, 1, 50), Mask: net.CIDRMask(24, 32)}
	addrB := net.IPNet{IP: net.IPv4(192, 168, 1, 87), Mask: net.CIDRMask(24, 32)}

	apply(&Lease{Address: addrA})
	if got := links.flushCountFor("eth0"); got != 0 {
		t.Fatalf("flush count after the first lease = %d, want 0 (nothing to flush yet)", got)
	}

	// A renewal that keeps the same address must not flush — that would
	// be a needless connectivity blip on every renewal.
	apply(&Lease{Address: addrA})
	if got := links.flushCountFor("eth0"); got != 0 {
		t.Errorf("flush count after a same-address renewal = %d, want 0", got)
	}
	if addrs := links.addrsFor("eth0"); len(addrs) != 1 {
		t.Errorf("addresses on eth0 = %v, want exactly one (same address applied twice)", addrs)
	}

	// A lease landing on a different address must flush the old one
	// first, since AddAddr's AddrReplace would otherwise add alongside.
	apply(&Lease{Address: addrB})
	if got := links.flushCountFor("eth0"); got != 1 {
		t.Errorf("flush count after an address change = %d, want 1", got)
	}
	if addrs := links.addrsFor("eth0"); len(addrs) != 1 || addrs[0].String() != addrB.String() {
		t.Errorf("addresses on eth0 = %v, want exactly [%v]", addrs, addrB)
	}
}
