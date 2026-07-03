//go:build linux

package netup

import (
	"context"
	"fmt"
	"net"

	"github.com/insomniacslk/dhcp/dhcpv4/nclient4"
	"github.com/vishvananda/netlink"
)

// NewPlatform wires up the real, netlink- and DHCPv4-backed implementations
// of Links and DHCPClient.
func NewPlatform() *Platform {
	return &Platform{
		Links: netlinkLinks{},
		DHCP:  nclient4Client{},
	}
}

// netlinkLinks implements Links using github.com/vishvananda/netlink.
type netlinkLinks struct{}

func (netlinkLinks) SetUp(name string) error {
	link, err := netlink.LinkByName(name)
	if err != nil {
		return fmt.Errorf("looking up link %s: %w", name, err)
	}
	if err := netlink.LinkSetUp(link); err != nil {
		return fmt.Errorf("bringing up link %s: %w", name, err)
	}
	return nil
}

func (netlinkLinks) AddAddr(name string, addr net.IPNet) error {
	link, err := netlink.LinkByName(name)
	if err != nil {
		return fmt.Errorf("looking up link %s: %w", name, err)
	}
	if err := netlink.AddrReplace(link, &netlink.Addr{IPNet: &addr}); err != nil {
		return fmt.Errorf("assigning %s to %s: %w", addr.String(), name, err)
	}
	return nil
}

func (netlinkLinks) ReplaceDefaultRoute(name string, gw net.IP) error {
	link, err := netlink.LinkByName(name)
	if err != nil {
		return fmt.Errorf("looking up link %s: %w", name, err)
	}
	route := &netlink.Route{LinkIndex: link.Attrs().Index, Gw: gw}
	if err := netlink.RouteReplace(route); err != nil {
		return fmt.Errorf("setting default route via %s on %s: %w", gw, name, err)
	}
	return nil
}

// Watch reports every interface's operational state (existing at
// subscribe time, via ListExisting, as well as future changes) so netup
// notices both an interface that's already up when gosd-init starts
// watching, and one that appears or flaps later.
func (netlinkLinks) Watch(stop <-chan struct{}) (<-chan LinkEvent, error) {
	updates := make(chan netlink.LinkUpdate)
	if err := netlink.LinkSubscribeWithOptions(updates, stop, netlink.LinkSubscribeOptions{
		ListExisting: true,
	}); err != nil {
		return nil, fmt.Errorf("subscribing to link updates: %w", err)
	}

	events := make(chan LinkEvent)
	go func() {
		defer close(events)
		for {
			select {
			case <-stop:
				return
			case u, ok := <-updates:
				if !ok {
					return
				}
				attrs := u.Link.Attrs()
				ev := LinkEvent{
					Name: attrs.Name,
					Up:   attrs.OperState == netlink.OperUp,
				}
				select {
				case events <- ev:
				case <-stop:
					return
				}
			}
		}
	}()
	return events, nil
}

// nclient4Client implements DHCPClient using
// github.com/insomniacslk/dhcp/dhcpv4/nclient4. A fresh client (and raw
// socket) is created per call rather than held open across renewals, so a
// link flap or interface re-appearance never has to reason about a stale
// socket bound to a now-gone interface.
type nclient4Client struct{}

func (nclient4Client) Request(ctx context.Context, iface string) (*Lease, error) {
	c, err := nclient4.New(iface)
	if err != nil {
		return nil, fmt.Errorf("creating DHCP client on %s: %w", iface, err)
	}
	defer c.Close()

	lease, err := c.Request(ctx)
	if err != nil {
		return nil, fmt.Errorf("DHCP discover/request on %s: %w", iface, err)
	}
	return fromDHCPLease(lease), nil
}

func (nclient4Client) Renew(ctx context.Context, iface string, lease *Lease) (*Lease, error) {
	raw, ok := lease.raw.(*nclient4.Lease)
	if !ok || raw == nil {
		return nil, fmt.Errorf("lease on %s has no renewable state", iface)
	}

	c, err := nclient4.New(iface)
	if err != nil {
		return nil, fmt.Errorf("creating DHCP client on %s: %w", iface, err)
	}
	defer c.Close()

	renewed, err := c.Renew(ctx, raw)
	if err != nil {
		return nil, fmt.Errorf("DHCP renew on %s: %w", iface, err)
	}
	return fromDHCPLease(renewed), nil
}

// fromDHCPLease translates an nclient4 lease into netup's own Lease type.
// T1/T2 default to the RFC 2131 Section 4.4.5 fallbacks (0.5x / 0.875x the
// lease time) when the server didn't send explicit values.
func fromDHCPLease(lease *nclient4.Lease) *Lease {
	ack := lease.ACK

	result := &Lease{
		DNS:        ack.DNS(),
		ObtainedAt: lease.CreationTime,
		raw:        lease,
	}
	result.ExpireAfter = ack.IPAddressLeaseTime(0)
	result.RenewAfter = ack.IPAddressRenewalTime(result.ExpireAfter / 2)
	result.RebindAfter = ack.IPAddressRebindingTime(result.ExpireAfter * 7 / 8)

	mask := net.IPMask(ack.SubnetMask())
	if mask == nil {
		mask = ack.YourIPAddr.DefaultMask()
	}
	result.Address = net.IPNet{IP: ack.YourIPAddr, Mask: mask}

	if routers := ack.Router(); len(routers) > 0 {
		result.Gateway = routers[0]
	}
	return result
}
