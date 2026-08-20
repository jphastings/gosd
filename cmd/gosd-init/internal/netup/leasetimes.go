package netup

import "time"

// Bounds netup imposes on the lease timers a DHCP server hands it. T1
// (option 58), T2 (option 59) and the lease time (option 51) are all
// uint32 second counts read straight off the wire, and the entire renewal
// schedule is derived from them — so without bounds, any DHCP server that
// answers first on the LAN decides how often a board with no shell and no
// way to intervene rewrites its own addresses, routes and resolv.conf
// (bean gosd-8lw0).
const (
	// minLeaseTimer is the shortest interval between two DHCP
	// conversations the maintenance loop will hold on one interface.
	// RFC 2131 Section 4.4.5 already floors a client's own DHCPREQUEST
	// retransmissions in RENEWING/REBINDING at 60 seconds, so no
	// conforming server needs to hear from us more often than this.
	minLeaseTimer = 60 * time.Second

	// maxLeaseTimer caps how long a lease is held without re-confirming
	// it with the server. RFC 2131 Section 3.3 permits lease times up to
	// 0xffffffff seconds — 136 years, which as a time.Duration also
	// overflows the T2 fallback arithmetic into a negative offset (i.e.
	// a renewal permanently in the past). A day bounds both, and costs a
	// network handing out longer leases one extra DHCPREQUEST per day.
	maxLeaseTimer = 24 * time.Hour

	// defaultLeaseTime stands in for a lease time the server didn't
	// send. RFC 2131 Section 4.3.1 requires option 51 in every DHCPACK,
	// so its absence — like an explicit zero — is a broken server, not a
	// request to renew continuously; renewing on the cadence of a
	// typical lease keeps such a server usable without hammering it.
	defaultLeaseTime = time.Hour
)

// saneLeaseTimers returns the T1/T2/lease-time to schedule from, given the
// values a server supplied. Absent or nonsensical values are replaced by
// the RFC 2131 Section 4.4.5 fallbacks (half and seven-eighths of the lease
// time), everything is confined to [minLeaseTimer, maxLeaseTimer], and the
// T1 <= T2 <= lease-time ordering is restored, so a server cannot schedule
// a renewal that has already passed or one that never arrives.
func saneLeaseTimers(renew, rebind, expire time.Duration) (time.Duration, time.Duration, time.Duration) {
	if expire <= 0 {
		expire = defaultLeaseTime
	}
	expire = clampDuration(expire, minLeaseTimer, maxLeaseTimer)

	if renew <= 0 {
		renew = expire / 2
	}
	if rebind <= 0 {
		// Divided before multiplied: expire * 7 overflows int64 for the
		// lease times a hostile server can send.
		rebind = expire / 8 * 7
	}

	renew = clampDuration(renew, minLeaseTimer, expire)
	rebind = clampDuration(rebind, renew, expire)
	return renew, rebind, expire
}

func clampDuration(d, low, high time.Duration) time.Duration {
	switch {
	case d < low:
		return low
	case d > high:
		return high
	default:
		return d
	}
}

// leaseSchedule converts a lease's server-supplied timers into the times
// the maintenance loop acts on. It reports the first lease on an interface
// whose timers had to be corrected — a rogue or simply broken DHCP server
// is otherwise invisible on a device whose only diagnostic is the console —
// and stays quiet about every lease after that, since a server that offers
// unusable timers once offers them on every renewal too.
type leaseSchedule struct {
	deps   Deps
	iface  string
	warned bool
}

func newLeaseSchedule(deps Deps, iface string) *leaseSchedule {
	return &leaseSchedule{deps: deps, iface: iface}
}

// times reports when to renew lease, and when to fall back to rebinding if
// that renewal fails.
func (s *leaseSchedule) times(lease *Lease) (renewAt, rebindAt time.Time) {
	renew, rebind, expire := saneLeaseTimers(lease.RenewAfter, lease.RebindAfter, lease.ExpireAfter)

	if !s.warned && (renew != lease.RenewAfter || rebind != lease.RebindAfter || expire != lease.ExpireAfter) {
		s.warned = true
		s.deps.Log("%s: DHCP server offered unusable lease timers (renew %s, rebind %s, lease %s); using renew %s, rebind %s, lease %s instead",
			s.iface, lease.RenewAfter, lease.RebindAfter, lease.ExpireAfter, renew, rebind, expire)
	}

	return lease.ObtainedAt.Add(renew), lease.ObtainedAt.Add(rebind)
}
