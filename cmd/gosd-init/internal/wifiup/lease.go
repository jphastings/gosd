package wifiup

import (
	"errors"
	"net"

	"github.com/jphastings/gosd/cmd/gosd-init/internal/netup"
)

// onLeaseFor returns the callback netup.RunDHCP invokes for every lease
// obtained (initial and renewed) on iface: assign the address, set the
// default route, write resolv.conf, and mark the network up. This
// mirrors netup's own (unexported) onLeaseFor exactly — small enough that
// duplicating it here is simpler than exporting it from netup for a
// single caller. That includes the stale-address flush: the closure
// tracks the address it last applied and flushes before assigning one
// that differs (association loss already flushed whatever was there
// before — see watchAssociation), so AddAddr's AddrReplace never leaves a
// wlan0 with two stale addresses stacked up after a reconnect lands a new
// lease (bean gosd-1lx7). A renewal that keeps the same address skips the
// flush, so it causes no connectivity blip.
func onLeaseFor(deps Deps, iface string) func(*netup.Lease) {
	var current *net.IPNet
	return func(lease *netup.Lease) {
		if current != nil && current.String() != lease.Address.String() {
			if err := deps.Links.FlushAddrs(iface); err != nil {
				deps.Log("flushing stale addresses on %s failed: %v", iface, err)
			}
		}
		if err := deps.Links.AddAddr(iface, lease.Address); err != nil {
			deps.Log("assigning %s to %s failed: %v", lease.Address, iface, err)
			return
		}
		current = &lease.Address
		if lease.Gateway != nil {
			if err := deps.Links.ReplaceDefaultRoute(iface, lease.Gateway); err != nil {
				deps.Log("setting default route via %s on %s failed: %v", lease.Gateway, iface, err)
			}
		}
		if err := deps.WriteResolvConf(lease.DNS); err != nil {
			if errors.Is(err, netup.ErrNoDNSServers) {
				deps.Log("%s: lease had no DNS servers; keeping existing resolv.conf", iface)
			} else {
				deps.Log("writing resolv.conf failed: %v", err)
			}
		}
		if err := deps.MarkNetworkUp(iface); err != nil {
			deps.Log("marking network up failed: %v", err)
		}
		deps.Log("%s: lease %s via gateway %s (dns %v)", iface, lease.Address, lease.Gateway, lease.DNS)
	}
}
