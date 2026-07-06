package mdnsresponder

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"time"

	"github.com/pion/mdns/v2"
	"golang.org/x/net/dns/dnsmessage"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

// collisionProbeTimeout bounds how long NewServer waits, in a background
// goroutine, for a foreign answer to its own hostname+".local" query before
// concluding the name is currently free on this network. gosd-r796 locks
// hostname-collision handling as log-only in v0.2 — no automatic renaming —
// so this probe exists purely to produce that log line; it never blocks
// startup or changes what NewServer returns.
const collisionProbeTimeout = 3 * time.Second

// NewServer starts a pion/mdns responder answering hostname+".local" on
// every interface that's up right now: passing nil Interfaces to
// mdns.Config makes it call net.Interfaces() itself and filter to FlagUp
// (excluding loopback, since IncludeLoopback is left false below), which is
// exactly the "on all up interfaces" behavior the bean specifies — gosd-init
// doesn't need to enumerate interfaces itself.
//
// This is the only production implementation of NewServerFunc; wired
// directly in main.go (no Platform/NewPlatform indirection, unlike
// netup/wifiup/timesync) because — like timesync's beevikClient — nothing
// here is a Linux-only syscall: pion/mdns is pure Go over UDP multicast
// sockets, so it needs no platform_linux.go/platform_other.go split.
func NewServer(hostname string, log func(format string, args ...any)) (Server, error) {
	fqdn := hostname + ".local"

	pc4, err4 := listenMulticast4()
	pc6, err6 := listenMulticast6()
	if pc4 == nil && pc6 == nil {
		return nil, fmt.Errorf("mdns: opening multicast sockets failed (IPv4: %v; IPv6: %v)", err4, err6)
	}
	if err4 != nil {
		log("mdns: IPv4 multicast unavailable, answering AAAA only: %v", err4)
	}
	if err6 != nil {
		log("mdns: IPv6 multicast unavailable, answering A only: %v", err6)
	}

	conn, err := mdns.Server(pc4, pc6, &mdns.Config{
		Name:       "gosd-init",
		LocalNames: []string{fqdn},
	})
	if err != nil {
		return nil, fmt.Errorf("starting mDNS responder for %s: %w", fqdn, err)
	}

	go probeForCollision(conn, fqdn, log, localInterfaceAddrs)

	return conn, nil
}

// listenMulticast4 opens the IPv4 mDNS multicast socket. Failing to bind
// isn't necessarily fatal to NewServer as a whole: a v6-only network still
// gets useful AAAA answers.
func listenMulticast4() (*ipv4.PacketConn, error) {
	addr, err := net.ResolveUDPAddr("udp4", mdns.DefaultAddressIPv4)
	if err != nil {
		return nil, err
	}
	l, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return nil, err
	}
	return ipv4.NewPacketConn(l), nil
}

// listenMulticast6 opens the IPv6 mDNS multicast socket. See
// listenMulticast4: failing to bind this alone isn't fatal either.
func listenMulticast6() (*ipv6.PacketConn, error) {
	addr, err := net.ResolveUDPAddr("udp6", mdns.DefaultAddressIPv6)
	if err != nil {
		return nil, err
	}
	l, err := net.ListenUDP("udp6", addr)
	if err != nil {
		return nil, err
	}
	return ipv6.NewPacketConn(l), nil
}

// collisionQuerier is the subset of *mdns.Conn that probeForCollision
// needs; *mdns.Conn satisfies it, and tests supply a fake so the
// self-conflict logic (gosd-90ir) can be exercised without real sockets.
type collisionQuerier interface {
	QueryAddr(ctx context.Context, name string) (dnsmessage.ResourceHeader, netip.Addr, error)
}

// probeForCollision issues a single mDNS query for fqdn shortly after
// startup and logs an answer that genuinely came from another host on the
// network. No answer within collisionProbeTimeout is the expected, silent,
// common case and is not logged.
//
// The query and this package's own responder share a single *mdns.Conn (the
// pion/mdns API gives no way to query without also answering on the same
// socket), so a reply to our own question always carries our own current
// address. Normally that self-answer never reaches us: IncludeLoopback is
// left false above, so pion/mdns never enables the socket's multicast
// loopback. But under qemu's user-mode networking (slirp), the virtual
// switch reflects the guest's own multicast traffic straight back to the
// guest regardless of that socket option — the reflection happens outside
// the guest's network stack entirely, in the host's NAT layer — so the
// probe can "hear" its own announcement and misreport it as a conflict
// (gosd-90ir). ownAddrs is queried fresh after the answer arrives, rather
// than once at startup, since DHCP may have handed us a new address between
// NewServer being called and the probe answer arriving.
func probeForCollision(querier collisionQuerier, fqdn string, log func(format string, args ...any), ownAddrs func() ([]netip.Addr, error)) {
	ctx, cancel := context.WithTimeout(context.Background(), collisionProbeTimeout)
	defer cancel()

	_, addr, err := querier.QueryAddr(ctx, fqdn)
	if err != nil {
		return
	}

	if isOwnAddress(addr, ownAddrs) {
		return
	}

	log("mdns: %s is already being answered by another host at %s; gosd-init does not auto-rename in v0.2 (log-only, see gosd-r796) — pick a unique hostname to resolve this", fqdn, addr)
}

// isOwnAddress reports whether addr belongs to one of this host's current
// network interfaces. A failure to enumerate interfaces fails open (treats
// addr as foreign) rather than swallowing a possibly-genuine conflict.
func isOwnAddress(addr netip.Addr, ownAddrs func() ([]netip.Addr, error)) bool {
	addrs, err := ownAddrs()
	if err != nil {
		return false
	}
	addr = addr.Unmap()
	for _, a := range addrs {
		if a.Unmap() == addr {
			return true
		}
	}
	return false
}

// localInterfaceAddrs lists the IP addresses bound to every local network
// interface at the moment it's called. It's a func value (not called
// directly by probeForCollision) purely so tests can substitute a fake set
// of addresses without touching the real network stack.
func localInterfaceAddrs() ([]netip.Addr, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	var addrs []netip.Addr
	for _, ifc := range ifaces {
		ifcAddrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range ifcAddrs {
			var ip net.IP
			switch v := a.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil {
				continue
			}
			if ipAddr, ok := netip.AddrFromSlice(ip); ok {
				addrs = append(addrs, ipAddr)
			}
		}
	}
	return addrs, nil
}
