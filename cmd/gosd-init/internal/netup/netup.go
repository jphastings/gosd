// Package netup brings up wired networking after /app has already been
// launched: link up, DHCPv4 discovery with retry/backoff, lease renewal,
// DNS, and reaction to link flaps (cable unplug/replug).
//
// Following the style established by the boot package, every
// side-effecting dependency (netlink, the DHCPv4 client, the clock, file
// writes) sits behind a thin interface (Links, DHCPClient, Clock, plus a
// couple of func fields on Deps), so the retry/backoff/renewal/flap state
// machine in this file and in dhcp.go is fully unit-tested with fakes on
// any OS. The real, Linux-syscall/netlink/DHCP-backed implementations live
// in platform_linux.go behind a "linux" build tag; platform_other.go stubs
// them out so `go test ./...` still passes on macOS.
package netup

import (
	"context"
	"net"
)

// Deps bundles every dependency the networking state machine needs.
// Production wiring (main.go) supplies Platform's real implementations;
// tests supply fakes.
type Deps struct {
	Links Links
	DHCP  DHCPClient
	Clock Clock

	// NewBackoff creates the retry/backoff strategy used for DHCP
	// discovery attempts. A func rather than a shared *Backoff so every
	// interface that comes up (including one that flaps down and back)
	// starts its own backoff sequence from scratch.
	NewBackoff func() *Backoff

	// WriteResolvConf overwrites the resolver config with the DNS
	// servers from a lease. See resolvconf.go for the writable-/etc
	// design choice.
	WriteResolvConf func(dns []net.IP) error

	// MarkNetworkUp and ClearNetworkUp create/remove the
	// /run/gosd/network-up marker file (empty-file existence check) that
	// the rest of gosd-init (and eventually /app) can use to tell
	// whether an address is currently assigned.
	MarkNetworkUp  func() error
	ClearNetworkUp func() error

	// Log records what the networking state machine is doing. Never
	// nil in production (wired to boot's console logger).
	Log func(format string, args ...any)
}

// Options holds the per-boot behavior knobs for networking bring-up.
type Options struct {
	// Stop, if non-nil, ends networking bring-up when closed. Production
	// leaves this nil so it runs for the life of the process, as PID 1
	// requires; tests set it to bound the otherwise-infinite watch loop.
	Stop <-chan struct{}
}

// Run brings up `lo`, then watches for wired interfaces (name matching
// eth*/end*/enp*) appearing or changing carrier state, running the DHCPv4
// state machine on each one that comes up and tearing it down when the
// link goes down. It only returns when opts.Stop is closed or the
// underlying link watch itself fails to start.
//
// Run is meant to be launched in its own goroutine (see boot.Deps.
// StartNetworking): per the locked behavior, network bring-up must never
// block or delay /app's start.
func Run(deps Deps, opts Options) {
	if err := deps.Links.SetUp("lo"); err != nil {
		deps.Log("bringing up lo failed: %v", err)
	}

	events, err := deps.Links.Watch(opts.Stop)
	if err != nil {
		deps.Log("watching for network interfaces failed: %v", err)
		return
	}

	active := map[string]context.CancelFunc{}
	defer func() {
		for _, cancel := range active {
			cancel()
		}
	}()

	for {
		select {
		case <-opts.Stop:
			return
		case ev, ok := <-events:
			if !ok {
				return
			}
			if !isWiredInterface(ev.Name) {
				continue
			}
			handleLinkEvent(deps, ev, active)
		}
	}
}

// handleLinkEvent starts or stops the DHCP state machine for an interface
// in response to it coming up (administratively + carrier) or going down,
// tracking the running instance's cancel func in active so a flap doesn't
// leak goroutines or start a second DHCP loop on top of an existing one.
func handleLinkEvent(deps Deps, ev LinkEvent, active map[string]context.CancelFunc) {
	_, running := active[ev.Name]

	switch {
	case ev.Up && !running:
		if err := deps.Links.SetUp(ev.Name); err != nil {
			deps.Log("bringing up %s failed: %v", ev.Name, err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		active[ev.Name] = cancel
		iface := ev.Name
		go func() {
			if err := RunDHCP(ctx, deps, iface, onLeaseFor(deps, iface)); err != nil && ctx.Err() == nil {
				deps.Log("DHCP on %s stopped unexpectedly: %v", iface, err)
			}
		}()

	case !ev.Up && running:
		cancel := active[ev.Name]
		cancel()
		delete(active, ev.Name)
		// Not explicitly required by the bean (which only says to
		// write the marker on lease assignment), but leaving a stale
		// network-up marker after the cable is pulled would be
		// actively misleading to anything that checks it, so we clear
		// it on link-down too.
		if err := deps.ClearNetworkUp(); err != nil {
			deps.Log("clearing network-up marker for %s failed: %v", ev.Name, err)
		}
		deps.Log("%s went down; DHCP stopped, will resume automatically when the link returns", ev.Name)
	}
}

// onLeaseFor returns the callback RunDHCP invokes for every lease obtained
// (initial and renewed) on iface: assign the address, set the default
// route, write resolv.conf, and mark the network up.
func onLeaseFor(deps Deps, iface string) func(*Lease) {
	return func(lease *Lease) {
		if err := deps.Links.AddAddr(iface, lease.Address); err != nil {
			deps.Log("assigning %s to %s failed: %v", lease.Address, iface, err)
			return
		}
		if lease.Gateway != nil {
			if err := deps.Links.ReplaceDefaultRoute(iface, lease.Gateway); err != nil {
				deps.Log("setting default route via %s on %s failed: %v", lease.Gateway, iface, err)
			}
		}
		if err := deps.WriteResolvConf(lease.DNS); err != nil {
			deps.Log("writing resolv.conf failed: %v", err)
		}
		if err := deps.MarkNetworkUp(); err != nil {
			deps.Log("marking network up failed: %v", err)
		}
		deps.Log("%s: lease %s via gateway %s (dns %v)", iface, lease.Address, lease.Gateway, lease.DNS)
	}
}

// isWiredInterface reports whether name matches one of the predictable or
// legacy Linux wired-Ethernet naming schemes the bean specifies: eth*
// (legacy), end* / enp* (systemd/u-boot predictable network interface
// names).
func isWiredInterface(name string) bool {
	for _, prefix := range [...]string{"eth", "end", "enp"} {
		if len(name) > len(prefix) && name[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}
