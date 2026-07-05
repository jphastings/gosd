package mdnsresponder

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/pion/mdns/v2"
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

	go probeForCollision(conn, fqdn, log)

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

// probeForCollision issues a single mDNS query for fqdn shortly after
// startup and logs — but never acts on — an answer from another host.
// gosd-init doesn't join its own multicast loopback (conn was constructed
// with IncludeLoopback left false), so any answer this receives genuinely
// came from a different host on the network, not an echo of gosd-init's own
// responder. No answer within collisionProbeTimeout is the expected,
// silent, common case and is not logged.
func probeForCollision(conn *mdns.Conn, fqdn string, log func(format string, args ...any)) {
	ctx, cancel := context.WithTimeout(context.Background(), collisionProbeTimeout)
	defer cancel()

	_, addr, err := conn.QueryAddr(ctx, fqdn)
	if err != nil {
		return
	}

	log("mdns: %s is already being answered by another host at %s; gosd-init does not auto-rename in v0.2 (log-only, see gosd-r796) — pick a unique hostname to resolve this", fqdn, addr)
}
