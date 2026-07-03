package netup

import (
	"context"
	"net"
	"time"
)

// Lease is the subset of a DHCPv4 lease netup acts on, deliberately
// decoupled from any specific DHCP client library's types so the state
// machine in dhcp.go and netup.go can be constructed and asserted on by
// tests without ever importing github.com/insomniacslk/dhcp.
type Lease struct {
	// Address is the assigned client address plus the subnet mask from
	// the lease.
	Address net.IPNet
	// Gateway is the first router from the lease's Router option, or nil
	// if the server didn't offer one.
	Gateway net.IP
	// DNS is the lease's Domain Name Server option.
	DNS []net.IP

	// ObtainedAt is when this lease (or, for a renewal, the renewed
	// lease) was granted; RenewAfter/RebindAfter/ExpireAfter are
	// durations relative to it (the lease's T1/T2/lease-time), per
	// RFC 2131 Section 4.4.5.
	ObtainedAt  time.Time
	RenewAfter  time.Duration
	RebindAfter time.Duration
	ExpireAfter time.Duration

	// raw is the originating DHCP library's lease value (e.g.
	// *nclient4.Lease on Linux), needed by the DHCPClient implementation
	// that produced this Lease to build a subsequent Renew request. It's
	// opaque to everything else in this package by design: fakes used in
	// tests can leave it nil or set any placeholder value, since nothing
	// but the real DHCPClient implementation ever inspects it.
	raw any //nolint:unused // only referenced from platform_linux.go; the unused linter can't see it when run with GOOS=darwin
}

// DHCPClient performs the DHCPv4 conversations netup needs. Implementations
// must be safe to use from a single goroutine at a time per iface (netup
// never calls into the same iface's client concurrently).
type DHCPClient interface {
	// Request performs a full Discover-Offer-Request-Ack handshake on
	// iface and returns the resulting lease.
	Request(ctx context.Context, iface string) (*Lease, error)
	// Renew asks the server that issued lease to renew it. lease must be
	// a value previously returned by Request or Renew for the same
	// iface.
	Renew(ctx context.Context, iface string, lease *Lease) (*Lease, error)
}

// LinkEvent reports that a network interface now exists with the given
// name and administrative-up-plus-carrier state.
type LinkEvent struct {
	Name string
	// Up is true only when the interface is both administratively up
	// and has a carrier signal (cable plugged into a live switch port) —
	// i.e. it's genuinely usable, not merely present.
	Up bool
}

// Links performs the netlink operations netup needs: bringing interfaces
// up, configuring them once a DHCP lease is obtained, and watching for
// interfaces appearing or changing carrier state (including cable
// unplug/replug).
type Links interface {
	// SetUp brings name administratively up (equivalent to `ip link set
	// name up`). Safe to call on an already-up interface.
	SetUp(name string) error
	// AddAddr assigns addr to name.
	AddAddr(name string, addr net.IPNet) error
	// ReplaceDefaultRoute installs (or replaces) the default route via gw
	// on name.
	ReplaceDefaultRoute(name string, gw net.IP) error
	// Watch streams link add/change events until stop is closed. It must
	// report every interface that already exists at the time Watch is
	// called (not just ones that appear afterwards), since a wired
	// interface may already be present and up before gosd-init starts
	// watching.
	Watch(stop <-chan struct{}) (<-chan LinkEvent, error)
}

// Clock abstracts time so the DHCP lease/renewal/backoff timers can be
// driven deterministically in tests (and interrupted by link-flap or
// shutdown signals) without any real waiting.
type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}
