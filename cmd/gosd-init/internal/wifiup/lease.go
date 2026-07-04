package wifiup

import "github.com/jphastings/gosd/cmd/gosd-init/internal/netup"

// onLeaseFor returns the callback netup.RunDHCP invokes for every lease
// obtained (initial and renewed) on iface: assign the address, set the
// default route, write resolv.conf, and mark the network up. This
// mirrors netup's own (unexported) onLeaseFor exactly — small enough that
// duplicating it here is simpler than exporting it from netup for a
// single caller.
func onLeaseFor(deps Deps, iface string) func(*netup.Lease) {
	return func(lease *netup.Lease) {
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
