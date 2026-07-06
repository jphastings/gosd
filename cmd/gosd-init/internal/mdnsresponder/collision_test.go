package mdnsresponder

import (
	"context"
	"net/netip"
	"testing"

	"golang.org/x/net/dns/dnsmessage"
)

// fakeQuerier scripts a single QueryAddr outcome, standing in for the real
// *mdns.Conn so probeForCollision's own-answer filtering (gosd-90ir) can be
// exercised without real sockets.
type fakeQuerier struct {
	addr netip.Addr
	err  error
}

func (f fakeQuerier) QueryAddr(context.Context, string) (dnsmessage.ResourceHeader, netip.Addr, error) {
	return dnsmessage.ResourceHeader{}, f.addr, f.err
}

// fakeOwnAddrs returns a fixed address set, standing in for
// localInterfaceAddrs.
func fakeOwnAddrs(addrs ...netip.Addr) func() ([]netip.Addr, error) {
	return func() ([]netip.Addr, error) { return addrs, nil }
}

// TestProbeForCollisionIgnoresOwnAnswer reproduces gosd-90ir: under qemu's
// user-mode networking, the probe's own multicast question is reflected
// back by the slirp virtual switch and answered by gosd-init's own
// responder (sharing the same *mdns.Conn), so the "foreign" answer it
// receives actually carries the device's own current address. That must
// not be logged as a conflict.
func TestProbeForCollisionIgnoresOwnAnswer(t *testing.T) {
	self := netip.MustParseAddr("10.0.2.15")
	log := &testLog{}

	probeForCollision(fakeQuerier{addr: self}, "qemu-hello.local", log.Printf, fakeOwnAddrs(self))

	if log.contains("already being answered") {
		t.Errorf("logged a self-answer as a conflict: %v", log.snapshot())
	}
}

// TestProbeForCollisionLogsGenuineConflict is the counterpart: an answer
// from an address that isn't ours is a real second responder for the same
// name, and must still be logged.
func TestProbeForCollisionLogsGenuineConflict(t *testing.T) {
	self := netip.MustParseAddr("10.0.2.15")
	other := netip.MustParseAddr("10.0.2.42")
	log := &testLog{}

	probeForCollision(fakeQuerier{addr: other}, "qemu-hello.local", log.Printf, fakeOwnAddrs(self))

	if !log.contains("qemu-hello.local is already being answered by another host at 10.0.2.42") {
		t.Errorf("missing conflict log: %v", log.snapshot())
	}
}

// TestProbeForCollisionUsesCurrentAddressesNotAStartupSnapshot guards
// against a regression where ownAddrs is captured once when the responder
// starts: DHCP can hand the interface a new address between NewServer being
// called and the probe answer arriving, and the check must reflect the
// address set at answer time, not at startup.
func TestProbeForCollisionUsesCurrentAddressesNotAStartupSnapshot(t *testing.T) {
	newlyLeased := netip.MustParseAddr("10.0.2.99")
	log := &testLog{}

	calls := 0
	ownAddrs := func() ([]netip.Addr, error) {
		calls++
		// Simulates DHCP having replaced the address by the time the
		// probe answer arrives: whatever startup captured is stale,
		// this call must be the one that's consulted.
		return []netip.Addr{newlyLeased}, nil
	}

	probeForCollision(fakeQuerier{addr: newlyLeased}, "qemu-hello.local", log.Printf, ownAddrs)

	if calls == 0 {
		t.Fatal("ownAddrs was never consulted")
	}
	if log.contains("already being answered") {
		t.Errorf("logged a self-answer (post-DHCP-change address) as a conflict: %v", log.snapshot())
	}
}

// TestProbeForCollisionSkipsLoggingOnQueryError covers the existing,
// unrelated early-return: no answer within the timeout is the silent,
// common case.
func TestProbeForCollisionSkipsLoggingOnQueryError(t *testing.T) {
	log := &testLog{}

	probeForCollision(fakeQuerier{err: errBoom}, "qemu-hello.local", log.Printf, fakeOwnAddrs())

	if len(log.snapshot()) != 0 {
		t.Errorf("expected no log lines on query error, got: %v", log.snapshot())
	}
}
